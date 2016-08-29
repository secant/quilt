var image = "quilt/wordpress";

exports.create = function(n, db, memcd) {
    var master = db.master;
    var rep = db.replicas;
    var dks = new Docker(image)
        .withEnv({
            "MEMCACHED": memcd.children().join(","),
            "DB_MASTER": master.children().join(","),
            "DB_REPLICA": rep.children().join(","),
        })
        .replicate(n);
    var wp = new Label("wp", dks);
    connect(3306, wp, rep);
    connect(3306, wp, master);
    connect(11211, wp, memcd);
    return wp;
}
