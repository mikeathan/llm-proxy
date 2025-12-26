import { IncomingMessage, ServerResponse } from "http";

import { sendJson } from "./http/json_response";

type Handler = (req: IncomingMessage, res: ServerResponse) => void | Promise<void>;

export class Router {
  private readonly routes = new Map<string, Handler>();

  register(method: string, path: string, handler: Handler): void {
    const key = this.key(method, path);
    this.routes.set(key, handler);
  }

  async handle(req: IncomingMessage, res: ServerResponse): Promise<void> {
    if (!req.url) {
      sendJson(res, 400, { error: "missing url" });
      return;
    }

    const url = new URL(req.url, `http://${req.headers.host ?? "localhost"}`);
    const key = this.key(req.method ?? "GET", url.pathname);
    const handler = this.routes.get(key);

    if (!handler) {
      sendJson(res, 404, { error: "not found" });
      return;
    }

    await handler(req, res);
  }

  private key(method: string, path: string): string {
    return `${method.toUpperCase()} ${path}`;
  }
}
