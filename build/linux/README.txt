NOUS - Proof of Intelligence Cryptocurrency
============================================

Quick Start
-----------

1. Create a wallet:

   nous-cli createwallet

2. Start mining (connect to seed nodes):

   nousd -testnet -mine -rpc 127.0.0.1:9332 \
         -peers SEED_NODE_1:9333,SEED_NODE_2:9333

3. Check balance:

   nous-cli -rpc 127.0.0.1:9332 getbalance

4. Send NOUS:

   nous-cli -rpc 127.0.0.1:9332 send <address> <amount>

For more information visit: [whitepaper link]
