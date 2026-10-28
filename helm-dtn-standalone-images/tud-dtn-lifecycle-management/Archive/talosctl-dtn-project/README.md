\# TalosCTL-DTN Image Transfer Prototype



This project demonstrates an image transfer workflow from the Earth side to the Service VM side using a TalosCTL-DTN style architecture.



\## Goal



The goal is to transfer an image bundle automatically through DTN instead of manually copying files.



\## Architecture



Earth Side:

\- Prepare image payload

\- Add metadata

\- Create TalosCTL-DTN bundle

\- Send bundle using uD3TN AAP2 sender



DTN Layer:

\- uD3TN daemon

\- Bundle Protocol transport

\- Store-and-forward communication



Service VM Side:

\- Receive bundle using uD3TN AAP2 receiver

\- Save received bundle

\- Extract image payload and metadata



\## Commands Used



Start uD3TN daemon:



```bash

cd \~/ud3tn

./build/posix/ud3tn --eid dtn://earth/



Start receiver:

python3 python-ud3tn-utils/ud3tn\_utils/aap2/bin/aap2\_receive.py \\

&#x20; --socket ud3tn.aap2.socket \\

&#x20; --agentid service-vm \\

&#x20; --count 1 \\

&#x20; --output /mnt/c/users/hp/darksol-hands-on/talosctl-dtn-project/service-vm/small-received.tar \\

&#x20; -v



Send bundle:



cat dtn-transfer/small-bundle.tar | \\

python3 \~/ud3tn/python-ud3tn-utils/ud3tn\_utils/aap2/bin/aap2\_send.py \\

&#x20; --socket \~/ud3tn/ud3tn.aap2.socket \\

&#x20; --agentid earth-side \\

&#x20; dtn://earth/service-vm



Result:



Small bundle transfer was successful.

Received Bundle from 'dtn://earth/earth-side'

payload len = 112640

Expected amount of bundles received, terminating.



Extracted files:



small-image.tar

metadata.yaml

small-received.tar



Note



Talosctl alone does not transfer container images. In this prototype, TalosCTL-DTN represents the operational workflow, while uD3TN provides the actual DTN transport layer.



Large Docker image transfer requires chunking or fragmentation.

