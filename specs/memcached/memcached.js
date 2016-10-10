var image = "quilt/memcached";

function Memcached(n) {
    var cns = new Container(image).replicate(n);
    this.memcd = new Label("memcd", cns);

    this.deploy = function(deployment) {
        deployment.deploy(this.memcd);
    };
}

exports.Memcached = Memcached;
