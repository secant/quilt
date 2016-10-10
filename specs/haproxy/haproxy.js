var image = "quilt/haproxy"
var cfg = "/usr/local/etc/haproxy/haproxy.cfg";

function Haproxy(n, hosts) {
    var hostnames = hosts.children().join(",");
    var args = [hostnames, "haproxy", "-f", cfg];
    var cns = new Container(image, args)
        .replicate(n);
    this.hap = new Label("hap", cns);
    this.hap.connect(80, hosts);

    this.deploy = function(deployment) {
        deployment.deploy(this.hap);
    };
};

exports.Haproxy = Haproxy;
