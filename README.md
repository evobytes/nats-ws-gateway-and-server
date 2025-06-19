## nats-ws-gateway-and-server

An all-in-one NATS.io server c/w a WebSocket http server.

### Build (Linux / MacOS / WSL2)

Using a Linux/Unix system, pull the repo and

* Use **make** to run or build for your current processor architecture - i.e. X86 or Arm.

Note that Arm/MacOS builds are untested by be but assume to work.

#### Tested environment

This app was developed on Ubuntu 24.04 LTS Linux so no guarantees of completeness on other development platforms are made.

### Clients

Examples of various clients using WebSockets to connect are provided.

* C#
* Go
* Python
* Javascript in HTML

While the examples provided should work, they do not feature any security provision.
This should be the responsibility of the proxy environment.
Client apps running on the same host as the server app could use ws://127.1/ to connect.
