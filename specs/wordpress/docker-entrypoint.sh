#!/bin/bash
set -e

timestamp() {
    until ping -q -c1 localhost > /dev/null 2>&1; do
        sleep 0.5
    done
    date -u +%s > /tmp/boot_timestamp
}
timestamp &

# Set default values
WORDPRESS_DB_HOST="${DB_MASTER:-DB_MASTER_CHANGE_ME}"
WORDPRESS_DB_NAME="${DB_NAME:-wordpress}"
WORDPRESS_DB_USER="${DB_USER:-wordpress}"
WORDPRESS_DB_PASSWORD="${DB_PASSWORD:-wordpress}"

DI_REDIS_ACTIVE=false
DI_MEMCACHED_ACTIVE=false
DI_MYSQL_REPLICATION=false
if [ -n "$REDIS" ]; then
    DI_REDIS_ACTIVE=true
fi
if [ -n "$MEMCACHED" ]; then
    DI_MEMCACHED_ACTIVE=true
fi
if [ -n "$DB_REPLICA" ]; then
    DI_MYSQL_REPLICATION=true
fi

echo "trying to reach $WORDPRESS_DB_HOST"
until ping -q -c1 "$WORDPRESS_DB_HOST" > /dev/null 2>&1; do
    sleep 1
done
echo "successfully reached $WORDPRESS_DB_HOST"

if [[ "$1" == apache2* ]] || [ "$1" == php-fpm ]; then
	if [ -n "$MYSQL_PORT_3306_TCP" ]; then
		if [ -z "$WORDPRESS_DB_HOST" ]; then
			WORDPRESS_DB_HOST='mysql'
		else
			echo >&2 'warning: both WORDPRESS_DB_HOST and MYSQL_PORT_3306_TCP found'
			echo >&2 "  Connecting to WORDPRESS_DB_HOST ($WORDPRESS_DB_HOST)"
			echo >&2 '  instead of the linked mysql container'
		fi
	fi

	if [ -z "$WORDPRESS_DB_HOST" ]; then
		echo >&2 'error: missing WORDPRESS_DB_HOST and MYSQL_PORT_3306_TCP environment variables'
		echo >&2 '  Did you forget to --link some_mysql_container:mysql or set an external db'
		echo >&2 '  with -e WORDPRESS_DB_HOST=hostname:port?'
		exit 1
	fi

	# if we're linked to MySQL and thus have credentials already, let's use them
	: ${WORDPRESS_DB_USER:=${MYSQL_ENV_MYSQL_USER:-root}}
	if [ "$WORDPRESS_DB_USER" = 'root' ]; then
		: ${WORDPRESS_DB_PASSWORD:=$MYSQL_ENV_MYSQL_ROOT_PASSWORD}
	fi
	: ${WORDPRESS_DB_PASSWORD:=$MYSQL_ENV_MYSQL_PASSWORD}
	: ${WORDPRESS_DB_NAME:=${MYSQL_ENV_MYSQL_DATABASE:-wordpress}}

	if [ -z "$WORDPRESS_DB_PASSWORD" ]; then
		echo >&2 'error: missing required WORDPRESS_DB_PASSWORD environment variable'
		echo >&2 '  Did you forget to -e WORDPRESS_DB_PASSWORD=... ?'
		echo >&2
		echo >&2 '  (Also of interest might be WORDPRESS_DB_USER and WORDPRESS_DB_NAME.)'
		exit 1
	fi

	if ! [ -e index.php -a -e wp-includes/version.php ]; then
		echo >&2 "WordPress not found in $(pwd) - copying now..."
		if [ "$(ls -A)" ]; then
			echo >&2 "WARNING: $(pwd) is not empty - press Ctrl+C now if this is an error!"
			( set -x; ls -A; sleep 10 )
		fi
		tar cf - --one-file-system -C /usr/src/wordpress . | tar xf -
		echo >&2 "Complete! WordPress has been successfully copied to $(pwd)"
		if [ ! -e .htaccess ]; then
			# NOTE: The "Indexes" option is disabled in the php:apache base image
			cat > .htaccess <<-'EOF'
				# BEGIN WordPress
				<IfModule mod_rewrite.c>
				RewriteEngine On
				RewriteBase /
				RewriteRule ^index\.php$ - [L]
				RewriteCond %{REQUEST_FILENAME} !-f
				RewriteCond %{REQUEST_FILENAME} !-d
				RewriteRule . /index.php [L]
				</IfModule>
				# END WordPress
			EOF
			chown www-data:www-data .htaccess
		fi
	fi

	# TODO handle WordPress upgrades magically in the same way, but only if wp-includes/version.php's $wp_version is less than /usr/src/wordpress/wp-includes/version.php's $wp_version

	# version 4.4.1 decided to switch to windows line endings, that breaks our seds and awks
	# https://github.com/docker-library/wordpress/issues/116
	# https://github.com/WordPress/WordPress/commit/1acedc542fba2482bab88ec70d4bea4b997a92e4
	sed -ri 's/\r\n|\r/\n/g' wp-config*

	if [ ! -e wp-config.php ]; then
		awk '/^\/\*.*stop editing.*\*\/$/ && c == 0 { c = 1; system("cat") } { print }' wp-config-sample.php > wp-config.php <<'EOPHP'
// If we're behind a proxy server and using HTTPS, we need to alert Wordpress of that fact
// see also http://codex.wordpress.org/Administration_Over_SSL#Using_a_Reverse_Proxy
if (isset($_SERVER['HTTP_X_FORWARDED_PROTO']) && $_SERVER['HTTP_X_FORWARDED_PROTO'] === 'https') {
	$_SERVER['HTTPS'] = 'on';
}

EOPHP
		chown www-data:www-data wp-config.php
	fi

	# see http://stackoverflow.com/a/2705678/433558
	sed_escape_lhs() {
		echo "$@" | sed 's/[]\/$*.^|[]/\\&/g'
	}
	sed_escape_rhs() {
		echo "$@" | sed 's/[\/&]/\\&/g'
	}
	php_escape() {
		php -r 'var_export(('$2') $argv[1]);' "$1"
	}
	set_config() {
		key="$1"
		value="$2"
		var_type="${3:-string}"
		start="(['\"])$(sed_escape_lhs "$key")\2\s*,"
		end="\);"
		if [ "${key:0:1}" = '$' ]; then
			start="^(\s*)$(sed_escape_lhs "$key")\s*="
			end=";"
		fi
		sed -ri "s/($start\s*).*($end)$/\1$(sed_escape_rhs "$(php_escape "$value" "$var_type")")\3/" wp-config.php
	}

	set_config 'DB_HOST' "$WORDPRESS_DB_HOST"
	set_config 'DB_USER' "$WORDPRESS_DB_USER"
	set_config 'DB_PASSWORD' "$WORDPRESS_DB_PASSWORD"
	set_config 'DB_NAME' "$WORDPRESS_DB_NAME"

	# allow any of these "Authentication Unique Keys and Salts." to be specified via
	# environment variables with a "WORDPRESS_" prefix (ie, "WORDPRESS_AUTH_KEY")
	UNIQUES=(
		AUTH_KEY
		SECURE_AUTH_KEY
		LOGGED_IN_KEY
		NONCE_KEY
		AUTH_SALT
		SECURE_AUTH_SALT
		LOGGED_IN_SALT
		NONCE_SALT
	)
	for unique in "${UNIQUES[@]}"; do
		eval unique_value=\$WORDPRESS_$unique
		if [ "$unique_value" ]; then
			set_config "$unique" "$unique_value"
		else
			# if not specified, let's generate a random value
			current_set="$(sed -rn "s/define\((([\'\"])$unique\2\s*,\s*)(['\"])(.*)\3\);/\4/p" wp-config.php)"
			if [ "$current_set" = 'put your unique phrase here' ]; then
				set_config "$unique" "$(head -c1M /dev/urandom | sha1sum | cut -d' ' -f1)"
			fi
		fi
	done

	if [ "$WORDPRESS_TABLE_PREFIX" ]; then
		set_config '$table_prefix' "$WORDPRESS_TABLE_PREFIX"
	fi

	if [ "$WORDPRESS_DEBUG" ]; then
		set_config 'WP_DEBUG' 1 boolean
	fi

	TERM=dumb php -- "$WORDPRESS_DB_HOST" "$WORDPRESS_DB_USER" "$WORDPRESS_DB_PASSWORD" "$WORDPRESS_DB_NAME" <<'EOPHP'
<?php
// database might not exist, so let's try creating it (just to be safe)

$stderr = fopen('php://stderr', 'w');

list($host, $port) = explode(':', $argv[1], 2);

$maxTries = 1000000;
do {
	$mysql = new mysqli($host, $argv[2], $argv[3], '', (int)$port);
	if ($mysql->connect_error) {
		fwrite($stderr, "\n" . 'MySQL Connection Error: (' . $mysql->connect_errno . ') ' . $mysql->connect_error . "\n");
		--$maxTries;
		if ($maxTries <= 0) {
			exit(1);
		}
		sleep(3);
	}
} while ($mysql->connect_error);

if (!$mysql->query('CREATE DATABASE IF NOT EXISTS `' . $mysql->real_escape_string($argv[4]) . '`')) {
	fwrite($stderr, "\n" . 'MySQL "CREATE DATABASE" Error: ' . $mysql->error . "\n");
	$mysql->close();
	exit(1);
}

$mysql->close();
EOPHP
fi

# Have apache log to the built-in location but also to a file
sed -i -e 's|^CustomLog \(.*\) combined|CustomLog "\|/usr/bin/tee \1 /var/log/apache2.log" combined|'  /etc/apache2/apache2.conf

if ! sudo -u www-data wp-cli --path=/var/www/html core is-installed; then
    sudo -u www-data wp-cli --path=/var/www/html core install \
        --url="http://wordpress.di" \
        --title="Wordpress" \
        --admin_user="wordpress" \
        --admin_password="wordpress" \
        --admin_email="changeme@wordpress.com"
fi
if [ "$DI_REDIS_ACTIVE" = true ]; then
    sed -i -e "s|\(^.*That's all, stop editing! Happy blogging.*$\)|define('WP_REDIS_HOST', 'redis.di');\n\n\1|" '/var/www/html/wp-config.php'
    unzip -o -d /var/www/html/wp-content/plugins/ /usr/src/redis-cache.zip
    ln -s /var/www/html/wp-content/plugins/redis-cache/includes/object-cache.php /var/www/html/wp-content/object-cache.php
    sudo -u www-data wp-cli --path=/var/www/html plugin activate redis-cache
fi
if [ "$DI_MEMCACHED_ACTIVE" = true ]; then
    unzip -o -d /var/www/html/wp-content/plugins/ /usr/src/memcached.zip
    ln -s /var/www/html/wp-content/plugins/memcached/object-cache.php /var/www/html/wp-content/object-cache.php
    echo "extension=memcache.so" >> /usr/local/etc/php/php.ini

    memcache_servers=''
    savedIFS=$IFS
    IFS=','
    for server in $MEMCACHED; do
        memcache_servers+="\t\t'${server}:11211',\n"
    done
    IFS=$savedIFS

    memcache_lines="\$memcached_servers = array(\n\t'default' => array(\n${memcache_servers}\t)\n);\n"
    sed -i -e "s|\(^.*That's all, stop editing! Happy blogging.*$\)|${memcache_lines}\n\n\1|" '/var/www/html/wp-config.php'
fi
if [ "$DI_MYSQL_REPLICATION" = true ]; then
    DI_DB_CONFIG="/var/www/html/wp-content/plugins/hyperdb/db-config.php"

    # XXX We aren't using the exact official plugin because it's currently
    # broken. Swap this back in once they fix it.
    #unzip -o -d /var/www/html/wp-content/plugins/ /usr/src/hyperdb.zip
    cp -r /usr/src/hyperdb /var/www/html/wp-content/plugins/

    DBLINE_COUNT=0
    DI_REMOVE_LINE=false
    DI_TMP_CONFIG="/tmp/di_tmp_config"
    DI_MASTER_NOREAD=false
    while IFS='' read -r line || [[ -n "$line" ]]; do
        if [ "$line" = '$wpdb->add_database(array(' ]; then
            DBLINE_COUNT=$((DBLINE_COUNT + 1))
            if [ $DBLINE_COUNT -eq 2 ]; then
                DI_REMOVE_LINE=true
            fi
        fi
        if [ "$DI_REMOVE_LINE" = true ]; then
            if [ "$line" = '));' ]; then
                DI_REMOVE_LINE=false
            fi
            continue
        fi
        echo "$line" >> "$DI_TMP_CONFIG"
        if [ $DBLINE_COUNT -eq 1 ]; then
            if [ ! "$DI_MASTER_NOREAD" = true ]; then
                echo "	'read'     => 0," >> "$DI_TMP_CONFIG"
                DI_MASTER_NOREAD=true
            fi
        fi
    done < "$DI_DB_CONFIG"
    cat "$DI_TMP_CONFIG" > "$DI_DB_CONFIG"

    savedIFS=$IFS
    IFS=','
    for server in $DB_REPLICA; do
        cat <<EOF >> "$DI_DB_CONFIG"
\$wpdb->add_database(array(
	'host'     => '$server',
	'user'     => DB_USER,
	'password' => DB_PASSWORD,
	'name'     => DB_NAME,
	'write'    => 0,
));
EOF
    done
    IFS=$savedIFS

    ln -s "$DI_DB_CONFIG" /var/www/html/db-config.php
    ln -s /var/www/html/wp-content/plugins/hyperdb/db.php /var/www/html/wp-content/db.php
fi

exec "$@"
