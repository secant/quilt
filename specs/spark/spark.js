var Image = "quilt/spark";

function SetImage(newImage) {
    Image = newImage;
}

function Spark(nMaster, nWorker, zookeeper) {
    var dkms = new Docker(Image, ["master"])
        .replicate(nMaster);
    
    if (zookeeper) {
        var zooHosts = zookeeper.children().join(",");
        for (var i = 0; i < nMaster; i++) {
            dkms[i].setEnv("ZOO", zooHosts);
        }
    }

    this.masters = new Label("spark-ms", dkms);
    
    var dkws = new Docker(Image, ["worker"])
        .withEnv({"MASTERS": this.masters.children().join(",")})
        .replicate(nWorker);
    this.workers = new Label("spark-wk", dkws);

    connect(7077, this.workers, this.workers);
    connect(7077, this.workers, this.masters);
    if (zookeeper) {
        connect(2181, this.masters, zookeeper);
    }
    
    this.job = function(command) {
        var cnt = this.masters.containers;
        for (var i = 0; i < cnt.length; i++) {
            cnt[i].env["JOB"] = command;
        }
    }
    this.public = function() {
        connect(new Port(8080), publicInternet, this.masters);
        connect(new Port(8081), publicInternet, this.workers);
    }

    this.exclusive = function() {
        place(this.masters, new LabelRule("exclusive", this.workers));
    }
}

exports.SetImage = SetImage;
exports.Spark = Spark;
