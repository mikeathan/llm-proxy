import { readFile } from "fs/promises";

export interface DeviceContextRepository {
  loadContext(): Promise<string>;
}

export class FileDeviceContextRepository implements DeviceContextRepository {
  private readonly path: string;

  constructor(path: string) {
    this.path = path;
  }

  async loadContext(): Promise<string> {
    return readFile(this.path, "utf8");
  }
}
