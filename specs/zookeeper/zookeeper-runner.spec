(import "zookeeper")

(define count 3)

(define zoo (zookeeper.New "zookeeper" count))

(define Namespace "CHANGE_ME")

// Defines the set of addresses that are allowed to access Quilt VMs.
(define AdminACL (list "local"))

(let ((cfg (list (provider "Amazon")
		 (region "us-west-1")
		 (size "m4.2xlarge")
		 (diskSize 32)
		 (githubKey "<YOUR_GITHUB_USERNAME>"))))
     (makeList 1 (machine (role "Master") cfg))
     (makeList count (machine (role "Worker") cfg)))
