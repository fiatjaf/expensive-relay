expensive-relay: $(shell find .. -name "*.go")
	go build -ldflags="-s -w" -o ./expensive-relay

deploy: expensive-relay
	ssh root@turgot 'systemctl stop expensive-relay'
	scp expensive-relay turgot:expensive-relay/expensive-relay
	ssh root@turgot 'systemctl start expensive-relay'
