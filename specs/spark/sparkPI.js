// Import spark.spec
var spark = require("github.com/NetSys/quilt/specs/spark/spark");

// Set the image to be Spark 1.5
spark.setImage("vfang/spark:1.5.2");

var deployment = createDeployment({
    // Using unique Namespaces will allow multiple Quilt instances to run on the
    // same cloud provider account without conflict.
    namespace: "kklin",
    // Defines the set of addresses that are allowed to access Quilt VMs.
    adminACL: ["local"],
});

// We will have three worker machines.
var nWorker = 3;

// Application
// sprk.exclusive enforces that no two Spark containers should be on the
// same node. sprk.public says that the containers should be allowed to talk
// on the public internet. sprk.job causes Spark to run that job when it
// boots.
var sprk = new spark.Spark(1, nWorker)
    .exclusive()
    .public()
    .job("run-example SparkPi");

// Infrastructure
var baseMachine = new Machine({
    provider: "Amazon",
    region: "us-west-1",
    size: "m4.large",
    diskSize: 32,
    sshKeys: githubKeys("kklin"),
});
deployment
    .deploy(baseMachine.asMaster())
    .deploy(baseMachine.asWorker().replicate(nWorker + 1))
    .deploy(sprk);

deployment.assert(sprk.masters.canReachFromPublic(), true);
deployment.assert(enough, true);
