var image = "quilt/mysql";

function Mysql(n) {
    this.master = createMaster();
    this.replicas = createReplicas(n, this.master);
    this.replicas.connect(3306, this.master);
    this.replicas.connect(22, this.master);

    this.deploy = function(deploy) {
        deployment.deploy(this.master);
        deployment.deploy(this.replicas);
    };
}

function createMaster() {
    var cn = new Container(image, ["--master", "1", "mysqld"]);
    return new Label("mysql-dbm", [cn]);
}

function createReplicas(n, master) {
    var cns = [];
    var mHost = master.children().join(",");
    for (i = 2; i < (n + 2); i++) {
        cns.push(new Container(image, ["--replica", mHost, "" + i, "mysqld"]));
    }
    return new Label("mysql-dbr", cns);
}

exports.Mysql = Mysql;
