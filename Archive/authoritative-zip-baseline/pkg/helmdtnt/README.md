# helmdtnt (plus a receiver application)
This component transmits an archive containing a Helm chart and its dependencies over a DTN.

The process is rather simple:
1. Connect to a µD3TN instance to use for the actual transmission
2. Read the archive as byte slice
3. Send it to µD3TN via AAP 2.0, which in turn sends it to the specified destination EID

On the receiver side, an application needs to run which basically reverses the operations executed by `helm-dtn`:
1. Connect to a µD3TN instance and register an agent to receive archives sent by `helm-dtn`
2. After receiving an archive: Untar the archive
3. Push all images contained in the archive to a local registry
4. Install the chart

If a custom dependency type needs "preprocessing" this has to happen between steps 3 and 4, as step 4 spawns a Pod/Deployment/... which may need this resource on startup.

This repo contains an example application using the aap2-client-go to connect to µD3TN, register the necessary agents and handle incoming archives. It can be used as a base if you want to implement some advanced resource handling, or as standalone application if you only need the image push functionality.
