containerIDCounter = 0;
deployment = new Deployment({});

var publicInternetName = "public";

function getDeployment() {
    deployment.vet();

    var containers = {};
    var labels = [];
    var connections = [];
    var placements = [];

    for (var i = 0 ; i < deployment.labels.length ; i++) {
        var label = deployment.labels[i];

        for (var j = 0 ; j < label.placements.length ; j++) {
            var placement = label.placements[j];
            placements.push({
                targetLabel: label.name,
                exclusive: placement.exclusive,

                otherLabel: placement.otherLabel || "",
                provider: placement.provider || "",
                size: placement.size || "",
                region: placement.region || "",
            });
        }

        for (var j = 0  ; j < label.connections.length ; j++) {
            var conn = label.connections[j];
            connections.push({
                from: label.name,
                to: conn.to.name,
                minPort: conn.minPort,
                maxPort: conn.maxPort,
            });
        }

        for (var j = 0  ; j < label.outgoingPublic.length ; j++) {
            var rng = label.outgoingPublic[j];
            connections.push({
                from: label.name,
                to: publicInternetName,
                minPort: rng.min,
                maxPort: rng.max,
            });
        }

        for (var j = 0  ; j < label.incomingPublic.length ; j++) {
            var rng = label.incomingPublic[j];
            connections.push({
                from: publicInternetName,
                to: label.name,
                minPort: rng.min,
                maxPort: rng.max,
            });
        }

        var ids = [];
        for (var j = 0 ; j < label.containers.length ; j++) {
            var container = label.containers[j];
            ids.push(container.id);
            containers[container.id] = container;
        }

        labels.push({
            name: label.name,
            ids: ids,
            annotations: label.annotations,
        });
    }

    return {
        machines: deployment.machines,
        invariants: deployment.invariants,
        containers: containers,
        labels: labels,
        connections: connections,
        placements: placements,

        namespace: deployment.namespace,
        adminACL: deployment.adminACL,
        maxPrice: deployment.maxPrice,
    }
}

function Deployment(deploymentOpts) {
    this.maxPrice = deploymentOpts.maxPrice || 0.0;
    this.namespace = deploymentOpts.namespace || "";
    this.adminACL = deploymentOpts.adminACL || [];

    this.machines = [];
    this.containers = {};
    this.labels = [];
    this.connections = [];
    this.placements = [];
    this.invariants = [];
}

Deployment.prototype.vet = function() {
    var labelMap = {};
    for (var i = 0 ; i < this.labels.length ; i++) {
        labelMap[this.labels[i].name] = true;
    }

    for (var i = 0 ; i < this.labels.length ; i++) {
        var label = this.labels[i];

        for (var j = 0 ; j < label.connections.length ; j++) {
            var to = label.connections[j].to.name;
            if (!labelMap[to]) {
                throw label.name + " has a connection to undeployed label: " + to;
            }
        }

        for (var j = 0 ; j < label.placements.length ; j++) {
            var otherLabel = label.placements[j].otherLabel;
            // If it's a MachineRule.
            if (otherLabel === undefined) {
                continue;
            }
            if (!labelMap[otherLabel]) {
                throw label.name + " has a placement in terms of an " +
                    "undeployed label: " + otherLabel;
            }
        }
    }
}

Deployment.prototype.deploy = function(toDeploy) {
    if (toDeploy.constructor !== Array) {
        toDeploy = [toDeploy];
    }

    for (var i = 0 ; i < toDeploy.length ; i++) {
        if (!toDeploy[i].deploy) {
            throw "can't deploy!";
        }
        toDeploy[i].deploy(this);
    }
    return this;
}

Machine.prototype.deploy = function(deployment) {
    deployment.machines.push(this);
}

Label.prototype.deploy = function(deployment) {
    deployment.labels.push(this);
}

Deployment.prototype.assert = function(rule, desired) {
    this.invariants.push(new Assertion(rule, desired));
}

function boxRange(x) {
    // Box raw integers into range.
    if (typeof x === "number") {
        x = new Range(x, x);
    }
    return x;
}

// XXX: Better name.
Label.prototype.connectToPublic = function(range) {
    range = boxRange(range);
    if (range.min != range.max) {
        throw "public internet cannot connect on port ranges";
    }
    this.outgoingPublic.push(range);
    return this;
}

// XXX: Better name.
Label.prototype.connectFromPublic = function(range) {
    range = boxRange(range);
    if (range.min != range.max) {
        throw "public internet cannot connect on port ranges";
    }
    this.incomingPublic.push(range);
    return this;
}

Label.prototype.connect = function(range, to) {
    range = boxRange(range);
    this.connections.push(new Connection(range, to));
    return this;
}

Label.prototype.place = function(rule) {
    this.placements.push(rule);
    return this;
}

function createDeployment(deploymentOpts) {
    deployment = new Deployment(deploymentOpts)
        return deployment;
}

function Machine(optionalArgs) {
    this.provider = optionalArgs.provider || "";
    this.role = optionalArgs.role || "";
    this.region = optionalArgs.region || "";
    this.size = optionalArgs.size || "";
    this.diskSize = optionalArgs.diskSize || 0;
    this.sshKeys = optionalArgs.sshKeys || [];

    var cpu = optionalArgs.cpu || new Range(0, 0);
    var ram = optionalArgs.ram || new Range(0, 0);

    // Autobox ints.
    if (cpu.constructor !== Range) {
        cpu = new Range(cpu, 0);
    }
    if (ram.constructor !== Range) {
        ram = new Range(ram, 0);
    }

    this.cpu = cpu;
    this.ram = ram;
}

Machine.prototype.clone = function() {
    // _.clone only creates a shallow copy, so we must clone sshKeys ourselves.
    var keyClone = _.clone(this.sshKeys);
    var cloned = _.clone(this);
    cloned.sshKeys = keyClone;
    return new Machine(cloned);
};

Machine.prototype.withRole = function(role) {
    var copy = this.clone();
    copy.role = role;
    return copy;
};

Machine.prototype.asWorker = function() {
    return this.withRole("Worker");
};

Machine.prototype.asMaster = function() {
    return this.withRole("Master");
};

Machine.prototype.replicate = function(n) {
    var res = [];
    for (var i = 0 ; i < n ; i++) {
        res.push(this.clone());
    }
    return res;
};

function Range(min, max) {
    this.min = min;
    this.max = max;
}

function Container(image, command) {
    this.id = ++containerIDCounter;
    this.image = image;
    this.command = command || [];
    this.env = {};
}

Container.prototype.clone = function() {
    var cloned = new Container(this.image, _.clone(this.command));
    cloned.env = _.clone(this.env);
    return cloned;
}

Container.prototype.replicate = function(n) {
    var res = [];
    for (var i = 0 ; i < n ; i++) {
        res.push(this.clone());
    }
    return res;
}

Container.prototype.withEnv = function(env) {
    this.env = env;
    return this;
}

var labelNameCount = {};
function uniqueLabelName(name) {
    if (!(name in labelNameCount)) {
        labelNameCount[name] = 0;
    }
    var count = ++labelNameCount[name];
    if (count == 1) {
        return name;
    }
    return name + labelNameCount[name];
}

function Label(name, containers) {
    this.name = uniqueLabelName(name);
    this.containers = containers;
    this.annotations = [];
    this.placements = [];

    this.connections = [];
    this.outgoingPublic = [];
    this.incomingPublic = [];
}

Label.prototype.hostname = function() {
    return this.name + ".q";
};

Label.prototype.children = function() {
    var res = [];
    for (var i = 1; i < this.containers.length + 1; i++) {
        res.push(i + "." + this.name + ".q");
    }
    return res;
};

Label.prototype.annotate = function(annotation) {
    this.annotations.push(annotation);
    return this;
};

Label.prototype.canReach = function(target) {
    return reachable(this.name, target.name);
};

Label.prototype.canReachPublic = function() {
    return reachable(this.name, publicInternetName);
};

Label.prototype.canReachFromPublic = function() {
    return reachable(publicInternetName, this.name);
};

Label.prototype.canReachACL = function(target) {
    return reachableACL(this.name, target.name);
};

Label.prototype.between = function(src, dst) {
    return between(src.name, this.name, dst.name);
};

Label.prototype.neighborOf = function(target) {
    return neighbor(this.name, target.name);
};

function LabelRule(exclusive, otherLabel) {
    this.exclusive = exclusive;
    this.otherLabel = otherLabel.name;
}

function MachineRule(exclusive, optionalArgs) {
    this.exclusive = exclusive;
    if (optionalArgs.provider) {
        this.provider = optionalArgs.provider;
    }
    if (optionalArgs.size) {
        this.size = optionalArgs.size;
    }
    if (optionalArgs.region) {
        this.region = optionalArgs.region;
    }
}

function Connection(ports, to) {
    this.minPort = ports.min;
    this.maxPort = ports.max;
    this.to = to;
}

function invariantType(form) {
    return function() {
        // Convert the arguments object into a real array. We can't simply use
        // Array.from because it isn't defined in Otto.
        var nodes = [];
        for (var i = 0 ; i < arguments.length ; i++) {
            nodes.push(arguments[i]);
        }

        return {
            form: form,
            nodes: nodes,
        }
    }
}

var enough = { form: "enough" };
var between = invariantType("between");
var neighbor = invariantType("reachDirect");
// XXX: Should reachACL and reach be separate?
var reachableACL = invariantType("reachACL");
var reachable = invariantType("reach");

function Assertion(invariant, desired) {
    this.form = invariant.form;
    this.nodes = invariant.nodes;
    this.target = desired;
}

function Port(p) {
    return new Range(p, p);
}

PortRange = Range;
