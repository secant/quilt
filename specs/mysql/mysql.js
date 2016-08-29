var image = "quilt/mysql";

exports.Create = function(n) {
    var master = createMaster();
    var replicas = createReplicas(n, master);
    connect(3306, replicas, master);
    connect(22, replicas, master);
    return {
        master: master,
        replicas: replicas
    };
};

function createMaster() {
    var dk = new Docker(image, ["--master", "1", "mysqld"]);
    return new Label("mysql-dbm", [dk]);
}

function createReplicas(n, master) {
    var dks = [];
    var mHost = master.children().join(",");
    for (i = 2; i < (n + 2); i++) {
        dks.push(new Docker(image, ["--replica", mHost, "" + i, "mysqld"]));
    }
    return new Label("mysql-dbr", dks);
}
