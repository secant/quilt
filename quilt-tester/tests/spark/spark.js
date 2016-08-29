var spark = require("github.com/NetSys/quilt/specs/spark/spark");
var infrastructure = require("github.com/NetSys/quilt/quilt-tester/config/infrastructure");

// Set the image to be Spark 1.5
spark.setImage("vfang/spark:1.5.2");

// Application
// sprk.exclusive enforces that no two Spark containers should be on the
// same node. sprk.public says that the containers should be allowed to talk
// on the public internet. sprk.job causes Spark to run that job when it
// boots.
var sprk = new spark.Spark(1, nWorker)
    .exclusive()
    .public()
    .job("run-example SparkPi");

var deployment = createDeployment({})
    .deploy(infrastructure)
    .deploy(sprk);

deployment.assert(sprk.masters.canReachFromPublic(), true);
deployment.assert(enough, true);
