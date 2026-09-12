import assert from "node:assert/strict";
import { test } from "node:test";
import { allowProxyOrigin, guardRpcProxy, type RpcProxy } from "../rpc-proxy.ts";

test("same-origin vite pages are allowed", () => {
  assert.equal(allowProxyOrigin("http://127.0.0.1:5173", "127.0.0.1:5173"), true);
  assert.equal(allowProxyOrigin("http://127.0.0.1:4173", "127.0.0.1:4173"), true);
  assert.equal(allowProxyOrigin(undefined, "127.0.0.1:5173"), true);
});

test("foreign pages cannot use the vite proxy", () => {
  assert.equal(allowProxyOrigin("https://attacker.example", "127.0.0.1:5173"), false);
  assert.equal(allowProxyOrigin("http://127.0.0.1:5173", "127.0.0.1:4173"), false);
  assert.equal(allowProxyOrigin("null", "127.0.0.1:5173"), false);
});

class FakeProxy implements RpcProxy {
  listeners = new Map<string, (...args: any[]) => void>();

  on(event: string, listener: (...args: any[]) => void): void {
    this.listeners.set(event, listener);
  }

  emit(event: string, ...args: any[]): void {
    this.listeners.get(event)?.(...args);
  }
}

test("dev and preview rewrite origin only after the original source is allowed", () => {
  const proxy = new FakeProxy();
  guardRpcProxy(proxy);
  const headers: Record<string, string> = {};
  let destroyed = false;
  proxy.emit(
    "proxyReqWs",
    {
      setHeader(name: string, value: string) {
        headers[name] = value;
      },
      destroy() {
        destroyed = true;
      },
    },
    { headers: { origin: "http://127.0.0.1:5173", host: "127.0.0.1:5173" } },
    { write() {}, destroy() {} },
  );
  assert.equal(headers.Origin, "http://127.0.0.1:8888");
  assert.equal(destroyed, false);

  const attackerHeaders: Record<string, string> = {};
  let attackerDestroyed = false;
  let status = "";
  proxy.emit(
    "proxyReqWs",
    {
      setHeader(name: string, value: string) {
        attackerHeaders[name] = value;
      },
      destroy() {
        attackerDestroyed = true;
      },
    },
    { headers: { origin: "https://attacker.example", host: "127.0.0.1:5173" } },
    {
      write(data: string) {
        status = data.split("\r\n")[0];
      },
      destroy() {},
    },
  );
  assert.equal(attackerHeaders.Origin, undefined);
  assert.equal(attackerDestroyed, true);
  assert.equal(status, "HTTP/1.1 403 Forbidden");
});

test("preview proxy uses the same origin guard", () => {
  const proxy = new FakeProxy();
  guardRpcProxy(proxy);
  const headers: Record<string, string> = {};
  proxy.emit(
    "proxyReq",
    {
      setHeader(name: string, value: string) {
        headers[name] = value;
      },
    },
    { headers: { origin: "http://127.0.0.1:4173", host: "127.0.0.1:4173" } },
    {},
  );
  assert.equal(headers.Origin, "http://127.0.0.1:8888");

  let status = 0;
  proxy.emit(
    "proxyReq",
    { setHeader() {} },
    { headers: { origin: "https://attacker.example", host: "127.0.0.1:4173" } },
    {
      writeHead(code: number) {
        status = code;
      },
      end() {},
    },
  );
  assert.equal(status, 403);
});
