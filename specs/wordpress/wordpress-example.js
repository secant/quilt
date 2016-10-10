var Wordpress = require("github.com/NetSys/quilt/specs/wordpress/wordpress");
var Memcached = require("github.com/NetSys/quilt/specs/memcached/memcached");
var Mysql = require("github.com/NetSys/quilt/specs/mysql/mysql");
var Haproxy = require("github.com/NetSys/quilt/specs/haproxy/haproxy");
var Spark = require("github.com/NetSys/quilt/specs/spark/spark");

var memcd = new Memcached.Memcached(3);
var db = new Mysql.Mysql(2);
var spark = new Spark.Spark(1, 4); // 1 Master, 4 Workers
var wp = new Wordpress.Wordpress(4, db, memcd);
var hap = new Haproxy.Haproxy(2, wp.wp);

spark.workers.connect(7077, db.master);
hap.hap.connectFromPublic(80);

// Infrastructure
var deployment = createDeployment({
    nasmespace: "vivian-wp",
    adminACL: ["local"],
});

var nWorker = 4;
var baseMachine = new Machine({
    provider: "Amazon",
    region: "us-west-1",
    size: "m4.large",
    diskSize: 32,
    keys: githubKeys("secant"),
});

deployment.deploy(baseMachine.asMaster())
    .deploy(baseMachine.asWorker().replicate(nWorker + 1))
    .deploy([memcd, db, spark, wp, hap]);
