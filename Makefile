.PHONY: proto clean


proto:
	@echo "Compiling protobuf"
	@rm -rf gen/proto
	@mkdir -p gen/proto
	@protoc --go_out=gen/ --go-grpc_out=gen/ \
        --go_opt=paths=source_relative --go-grpc_opt=paths=source_relative proto/raft.proto
	@echo "Done"


