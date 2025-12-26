import { resolve } from "path";

import { DeviceContextController } from "./controllers/device_context_controller";
import { FileDeviceContextRepository } from "./repositories/device_context_repository";
import { Router } from "./router";
import { HttpServer } from "./server";

const port = Number.parseInt(process.env.PORT ?? "3001", 10);
const samplePath = resolve(__dirname, "..", "..", "docs", "samples", "device_context.json");

const repository = new FileDeviceContextRepository(samplePath);
const controller = new DeviceContextController(repository);
const router = new Router();

router.register("GET", "/api/context/devices", (req, res) =>
  controller.getDeviceContext(req, res)
);

const server = new HttpServer(router);

server.listen(port, () => {
  console.log(`Mock server listening on http://localhost:${port}`);
  console.log("GET /api/context/devices");
  console.log(`Using sample data: ${samplePath}`);
});
