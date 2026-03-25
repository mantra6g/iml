# Architecture Overview

The Infrastructure Management Layer is a Kubernetes-native platform that allows management and orchestration
of network functions and programmable targets. It provides a unified interface for deploying, scaling, and managing
network functions across a variety of programmable targets, including P4-enabled switches, FPGAs, and SmartNICs.

<img
  style="display: block;
  margin-left: auto;
  margin-right: auto;
  width: 60%;"
  src="../../assets/system-level-view.png"
  alt="System level view"/>

To achieve this, we have designed IML as a collection of modular systems, each of them with their own responsibility.
As a result of this, the platform is made up of three core subsystems: 

- the [network function scheduling system](nf-scheduling-system.md), responsible
  for the scheduling and orchestration of network functions across the different targets;
 - the [P4Target addon system](p4target-addon-system.md), in charge of integrating both software and hardware 
  programmable targets into the platform;
 - and [IML's own networking plugin](iml-networking-plugin.md), which provides the necessary networking capabilities to 
  allow communication between network functions and external applications, such as IP address management and traffic
  steering.



