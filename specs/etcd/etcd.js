var image = "quilt/etcd";

function Etcd(n) {
    cns = new Container(image)
        .replicate(n);
    this.etcd = new Label("etcd", cns);
    var children = this.etcd.children();
    var peers = children.join(",");
    for (i = 0; i < cns.length; i++) {
        console.log(Array.isArray(cns[i].command));
        console.log(JSON.stringify(cns[i].command, null, 4));
        cns[i].env["PEERS"] = peers;
        cns[i].env["HOST"] = children[i];
    }
    this.etcd.connect(new PortRange(1000, 65535), this.etcd);
    
    this.deploy = function(deployment) {
        deployment.deploy(this.etcd);
    }
}

exports.Etcd = Etcd;
