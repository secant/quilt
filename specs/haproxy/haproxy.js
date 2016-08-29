var image = "quilt/haproxy"
var cfg = "/usr/local/etc/haproxy/haproxy.cfg";

exports.create = function(n, hosts) {
    var hostnames = hosts.children().join(",");
    var args = [hostnames, "haproxy", "-f", cfg];
    var dks = new Docker(image, args)
        .replicate(n);
    var hap = new Label("hap", dks);
    connect(80, hap, hosts);
    return hap;
};
