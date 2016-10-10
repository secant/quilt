// Create a new deployment.
// Using unique Namespaces will allow multiple Quilt instances to run on the
// same cloud provider account without conflict.
// Also defines the set of addresses that are allowed to access Quilt VMs.
var deployment = createDeployment({
    namespace: "CHANGE_ME",
    adminACL: ["local"],
});


// We will apply this configuration to each VM.
var baseMachine = new Machine({
    provider: "Amazon", // Supported providers include "Amazon", "Azure", "Google", and "Vagrant".
    keys: githubKeys("kklin"), // Change Me.
});

// Create Master and Worker Machines.
deployMasters(1, baseMachine);
deployWorkers(2, baseMachine);

// Create a Nginx Docker container, assigning it the label "web_tier".
var webTierLabel = new Label("web_tier", [new Container("nginx")]);

webTierLabel.connectFromPublic(80);
