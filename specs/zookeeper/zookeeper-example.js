var Zookeeper = require("github.com/NetSys/quilt/specs/zookeeper/zookeeper");

var n = 3;
var zoo = new Zookeeper.Zookeeper(n);
var deployment = createDeployment({
    namespace: "CHANGE_ME",
    adminACL: ["local"],
});

var baseMachine = new Machine({
    provider: "Amazon",
    region: "us-west-1",
    size: "m4.large",
    diskSize: 32,
    sshKeys: githubKeys("YOUR_GITHUB_KEY"),
});

deployment.deploy(baseMachine.asMaster())
    .deploy(baseMachine.asWorker().replicate(n))
    .deploy(zoo);
