var image = "quilt/zookeeper";

function Zookeeper(n) {
    var cns = new Container(image)
        .replicate(n);
    this.zoo = new Label("zookeeper", cns);
    var hostnames = this.zoo.children();
    for (var i = 0; i < cns.length; i++) {
        cns[i].command = ["" + (i + 1), hostnames.join(",")];
    }

    this.zoo.connect(new PortRange(1000, 65535), this.zoo);
    this.deploy = function(deployment) {
        deployment.deploy(this.zoo);
    }
}

exports.Zookeeper = Zookeeper;
