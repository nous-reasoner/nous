module github.com/nous-reasoner/nous-wallet/backend

go 1.24.0

replace nous => ../../

require (
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.4.1
	github.com/tyler-smith/go-bip39 v1.1.0
	golang.org/x/crypto v0.48.0
	nous v0.0.0-00010101000000-000000000000
)

require (
	go.etcd.io/bbolt v1.4.3 // indirect
	golang.org/x/sys v0.41.0 // indirect
)
