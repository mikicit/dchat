# Distributed Chat

## Description

Distributed chat on Go using gRPC and Protocol Buffers as a method of communication between nodes. 
The Hirschberg-Sinclair algorithm based on two-way ring topology was used as the leader election algorithm. 
This work was done as part of a semester project on Distributed Systems and Computing. 
The main purpose of the work was to learn about distributed algorithms.
Also for me personally it was interesting to get acquainted with Go, gRPC and Protocol Buffers. 
The project is not perfect and does not include handling all sorts of ichcases, but it should work in most cases.

## How to Run

### Install Protocol Buffers

    sudo apt install protobuf-compiler

### Install Go

    sudo apt install golang-go

### Install dependencies

    go get -u google.golang.org/grpc
    go get -u github.com/golang/protobuf/protoc-gen-go

### Compile proto files

    protoc -I=./proto --go_out=./proto --go_opt=paths=source_relative --go-grpc_out=require_unimplemented_servers=false:./proto --go-grpc_opt=paths=source_relative ./proto/*.proto

### Run chat

    go run .

or

    go build -o chat

### Logs

Default path for logs is `/var/log/dchat`. 
In this folder you can find logs for the entire app and for the chat messages.

## How to Use

### Commands



