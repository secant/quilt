var image = "quilt/wordpress";

function Wordpress(n, db, memcd) {
    var master = db.master;
    var rep = db.replicas;
    var cns = new Container(image)
        .withEnv({
            "MEMCACHED": memcd.memcd.children().join(","),
            "DB_MASTER": master.children().join(","),
            "DB_REPLICA": rep.children().join(","),
        })
        .replicate(n);
    this.wp = new Label("wp", cns);
    this.wp.connect(3306, rep);
    this.wp.connect(3306, master);
    this.wp.connect(11211, memcd.memcd);

    this.deploy = function(deployment) {
        deployment.deploy(this.wp);
    };
}

exports.Wordpress = Wordpress;
