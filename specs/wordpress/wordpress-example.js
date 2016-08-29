var Wordpress = require("github.com/NetSys/quilt/specs/wordpress/wordpress");
var Memcached = require("github.com/NetSys/quilt/specs/memcached/memcached");
var Mysql = require("github.com/NetSys/quilt/specs/mysql/mysql");
var Haproxy = require("github.com/NetSys/quilt/specs/haproxy/haproxy");
var Spark = require("github.com/NetSys/quilt/specs/spark/spark");

var memcd = Memcached.create(3);
var db = Mysql.create(2);
var spark = Spark.create(1, 4); // 1 Master, 4 Workers
var wp = Wordpress.create(4, db, memcd);
var hap = Haproxy.create(2, wp);

connect(7077, spark.workers, db.master);
connect(80, publicInternet, hap);

// Infrastructure
setNamespace("CHANGE_ME");

var nWorker = 4;
var baseMachine = new Machine({
    provider: "Amazon",
    region: "us-west-1",
    size: "m4.large",
    diskSize: 32,
    keys: githubKeys("CHANGE_ME"),
});

deployWorkers(nWorker + 1, baseMachine);
deployMasters(1, baseMachine);
