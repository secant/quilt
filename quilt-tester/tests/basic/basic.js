var infrastructure = require("github.com/NetSys/quilt/quilt-tester/config/infrastructure")

var deployment = createDeployment({}).deploy(infrastructure);

var nWorker = 1;
var red = new Label("red", new Docker("google/pause").replicate(nWorker));
var blue = new Label("blue", new Docker("google/pause").replicate(3 * nWorker));

var ports = new PortRange(1024
red.connect(ports, blue);
blue.connect(ports, red);
