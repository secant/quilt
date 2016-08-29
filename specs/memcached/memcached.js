var image = "quilt/memcached";

exports.create = function(n) {
    var dks = new Docker(image).replicate(n);
    return new Label("memcd", dks);
};
