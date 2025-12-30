import { IncomingMessage } from "http";

export class BadRequestError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "BadRequestError";
  }
}

export async function readJson<T>(req: IncomingMessage): Promise<T> {
  const chunks: Buffer[] = [];

  for await (const chunk of req) {
    if (typeof chunk === "string") {
      chunks.push(Buffer.from(chunk, "utf8"));
    } else {
      chunks.push(chunk);
    }
  }

  const body = Buffer.concat(chunks).toString("utf8").trim();
  if (!body) {
    throw new BadRequestError("empty request body");
  }

  return JSON.parse(body) as T;
}
