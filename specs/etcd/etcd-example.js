var Etcd = require("github.com/NetSys/quilt/specs/etcd/etcd");

var nWorker = 3;
var etcd = new Etcd.Etcd(nWorker);

// Infrastructure
var deployment = createDeployment({
    namespace: "vivian-etcd",
    adminACL: ["local"],
});

var baseMachine = new Machine({
    provider: "Amazon",
    region: "us-west-1",
    size: "m4.large",
    diskSize: 32,
    sshKeys: githubKeys("secant"),
});

deployment
    .deploy(baseMachine.asMaster())
    .deploy(baseMachine.asWorker().replicate(nWorker + 1))
    .deploy(etcd);
