
proto:
	protoc \
	-I . \
	-I ../protos \
	--go_out . --go_opt paths=source_relative \
	--go-grpc_out . --go-grpc_opt paths=source_relative \
	--validate_out "lang=go:." --validate_opt paths=source_relative \
	--grpc-gateway_out . --grpc-gateway_opt paths=source_relative \
	pb/hello/hello.proto

server:
	cd grpc-s && go mod tidy && go run .
