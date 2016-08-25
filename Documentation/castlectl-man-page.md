**NAME**  
castlectl - a command line client for working with a castle storage cluster

**SYNOPSIS**  
castlectl [global options] node ls  
castlectl [global options] node update [--location=\<location\>]  
castlectl [global options] volume create --name=\<volume-name\> --size=\<volume-size\>  
castlectl [global options] volume update [options]  
castlectl [global options] health  
...  

**DESCRIPTION**  
castlectl is a command line client for configuring and managing a castle storage cluster.  It is used to directly interact with the resources that make up the cluster such as nodes and volumes.  For example, the nodes along with their details and status can be examined and new volumes can be created and prepared for use, using the underlying storage fabric of the castle cluster.
	
A castle discovery service, which automatically tracks all cluster members and their locations, can be used instead of having to know and specify all members locations as command line options.

**COMMANDS**  
node - lists and manages the nodes that compose the castle storage cluster  
volume - creates and manages volumes using the underlying storage fabric of the castle cluster  
health - summarizes the health of the castle storage cluster, including any warning or error statuses  
[other commands?]    

**OPTIONS**  
Global options (see specific command help for command options):  
  --discovery-url  The URL of the castle discovery service  
  --discovery-dns The DNS entry of the castle discovery service

**FILES**  
None

**ENVIRONMENT  VARIABLES**  
CASTLE_DISCOVERY_URL - equivalent to --discovery-url, overridden by command line option  
CASTLE_DISCOVERY_DNS - equivalent to --discovery-dns, overridden by command line option  

**SEE ALSO**  
castled(8), castleblk(8)  

**COPYRIGHT**  
Copyright 2016 Quantum Corporation, licensed under (license TBD)...

**AUTHORS**  
Castle Team
