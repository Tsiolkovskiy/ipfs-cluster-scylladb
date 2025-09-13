IPFS Cluster provides data orchestration across a swarm of IPFS daemons by allocating, replicating and tracking a global pinset distributed among multiple peers.

There are 3 different applications:

A cluster peer application: ipfs-cluster-service, to be run along with kubo (go-ipfs) as a sidecar.
A client CLI application: ipfs-cluster-ctl, which allows easily interacting with the peer's HTTP API.
An additional "follower" peer application: ipfs-cluster-follow, focused on simplifying the process of configuring and running follower peers.
Table of Contents
Documentation
News & Roadmap
Install
Usage
Contribute
License
Documentation
Please visit https://ipfscluster.io/documentation/ to access user documentation, guides and any other resources, including detailed download and usage instructions.

News & Roadmap
We regularly post project updates to https://ipfscluster.io/news/ .

The most up-to-date Roadmap is available at https://ipfscluster.io/roadmap/ .

Install
Instructions for different installation methods (including from source) are available at https://ipfscluster.io/download .

Usage
Extensive usage information is provided at https://ipfscluster.io/documentation/ , including:

Docs for ipfs-cluster-service
Docs for ipfs-cluster-ctl
Docs for ipfs-cluster-follow
Contribute
PRs accepted. As part of the IPFS project, we have some contribution guidelines.

License
This library is dual-licensed under Apache 2.0 and MIT terms.
