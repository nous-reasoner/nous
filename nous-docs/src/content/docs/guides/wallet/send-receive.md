---
title: Send & Receive
description: How to send and receive NOUS.
---

## Receiving NOUS

1. Go to **Wallet** tab
2. Click **Receive**
3. Your active address is displayed with a QR code
4. Share the address or QR code with the sender
5. Click **Copy** to copy the address to clipboard

Incoming transactions appear in your balance after **1 block confirmation**. You can spend received NOUS immediately.

Mined NOUS (coinbase) requires **100 confirmations** (~4 hours) before it can be spent.

## Sending NOUS

1. Go to **Wallet** tab
2. Click **Send**
3. Enter the recipient's NOUS address
4. Enter the amount in NOUS
5. Set a network fee (default: 0.0001 NOUS)
6. Optionally add a message (stored on-chain via OP_RETURN)
7. Review the confirmation screen
8. Click **Send**

The transaction ID is displayed immediately after broadcast.

## Fees

- Fees are paid by the **sender**
- The recipient receives exactly the amount you specify
- Example: Sending 1 NOUS with 0.0001 fee costs you 1.0001 NOUS total
- Fees go to the miner who includes your transaction in a block

## Address Switching

You can manage multiple addresses:
- Go to **Addresses** to see all your addresses
- Click **Use** to switch the active address
- Balance and history are filtered by the active address
- The active address is also used for mining rewards
