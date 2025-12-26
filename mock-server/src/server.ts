import { createServer, Server } from "http";

import { Router } from "./router";

export class HttpServer {
  private readonly server: Server;

  constructor(router: Router) {
    this.server = createServer((req, res) => {
      void router.handle(req, res);
    });
  }

  listen(port: number, onReady: () => void): void {
    this.server.listen(port, onReady);
  }
}
