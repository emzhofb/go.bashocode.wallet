#!/bin/bash
export PATH=$PATH:$HOME/go/bin:/usr/local/bin:/opt/homebrew/bin
cd "$(dirname "$0")/../proto" || exit 1

echo "Generating protobuf Go stubs..."
protoc --go_out=. --go_opt=paths=source_relative \
       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
       auth/auth.proto \
       user/user.proto \
       wallet/wallet.proto \
       ledger/ledger.proto

echo "Protobuf generation completed successfully!"
