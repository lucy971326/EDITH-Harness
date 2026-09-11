import { defineConfig, type ProxyOptions } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { fileURLToPath, URL } from "node:url";
import { guardRpcProxy } from "./rpc-proxy.ts";

const rpcProxy: ProxyOptions = {
  target: "http://127.0.0.1:8889",
  ws: true,
  changeOrigin: true,
  configure(proxy) {
    guardRpcProxy(proxy);
  },
};

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) } },
  server: {
    host: "127.0.0.1",
    proxy: { "/rpc": rpcProxy },
  },
  preview: {
    host: "127.0.0.1",
    proxy: { "/rpc": rpcProxy },
  },
});
