// Import spark.spec
var Spark = require("spark");

// Set the image to be Spark 1.5
Spark.SetImage("vfang/spark:1.5.2");

// We will have three worker machines.
var nWorker = 3;

// Application
// sprk.exclusive enforces that no two Spark containers should be on the
// same node. sprk.public says that the containers should be allowed to talk
// on the public internet. sprk.job causes Spark to run that job when it
// boots.
var sprk = new Spark.Spark(1, nWorker);
sprk.exclusive()
sprk.public()
sprk.job("run-example SparkPi");

// Infrastructure

// Using unique Namespaces will allow multiple Quilt instances to run on the
// same cloud provider account without conflict.
setNamespace("CHANGE_ME");

// Defines the set of addresses that are allowed to access Quilt VMs.
setAdminACL(["local"]);

var baseMachine = new Machine({
    provider: "AmazonSpot",
    region: "us-west-1",
    size: "m4.large",
    diskSize: 32,
    keys: githubKeys("CHANGE_ME"),
});
deployWorkers(nWorker + 1, baseMachine);
deployMasters(1, baseMachine);

assert(new Reachable(publicInternet, sprk.masters), true);
assert(new Enough(), true);
