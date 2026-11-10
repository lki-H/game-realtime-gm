# Third party notices

The owner authorized MIT licensing for this repository's own code and documentation on 2026-11-10. That grant does not relicense dependencies, engines, container images or generated-code runtime libraries.

| Component | Declaration and license |
| --- | --- |
| Go toolchain | Go BSD-style license; module minimum and tested toolchain are documented separately |
| Gin, Gorilla WebSocket, go-sql-driver/mysql | MIT, BSD-2-Clause, MPL-2.0 respectively; exact revisions are in backend/go.mod and go.sum |
| golang-jwt/jwt, go-redis | MIT and BSD-2-Clause; retain upstream notices |
| golang.org/x packages | Go BSD-style license; exact modules are locked by go.sum |
| gRPC and Protobuf | Apache-2.0 for gRPC-Go, BSD-3-Clause for Protobuf; R5 module/lock files and generated file headers identify their versions |
| RabbitMQ Go client (amqp091-go) | BSD-2-Clause at the locked revision; RabbitMQ server distributions have their own notices |
| React, Vite, TypeScript | MIT, MIT, Apache-2.0; both frontend package locks identify exact packages |
| Unity | Unity's proprietary license and applicable engine/package terms; no Unity executable is included |
| MySQL, Redis and container base images | Upstream distribution/image terms apply; source licensing here does not license container payloads |

R5 generated Go/C# bindings retain generator headers. They originate from this repository's own reporting schema; the Protobuf/gRPC runtime terms remain separate. C# contract fixtures are checks of message compatibility and are not a claim of a cross-language RPC client.

Dependency archives, engine binaries, private credentials, database backups and precompiled players are not part of this source delivery. Before distributing an executable, inspect the exact dependency/image inventory and retain its applicable notices. The table is a navigation aid, not a replacement for upstream license texts.
