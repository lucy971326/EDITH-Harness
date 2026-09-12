const backendOrigin = "http://127.0.0.1:8888";

export type ProxyRequest = {
  setHeader: (name: string, value: string) => void;
  destroy?: () => void;
};

export type IncomingProxyReq = {
  headers?: Record<string, string | string[] | undefined>;
};

export type ProxyResponse = {
  writeHead?: (status: number) => void;
  end?: () => void;
};

export type UpgradeSocket = {
  write: (data: string) => void;
  destroy: () => void;
};

export type RpcProxy = {
  on(event: string, listener: (...args: any[]) => void): void;
};

// 原始 Origin 必须与当前 Vite 页的 Host 一致，通过后才改写成后台地址。
export function allowProxyOrigin(origin: string | undefined, host: string | undefined): boolean {
  if (origin == null || origin === "") return true;
  if (!host) return false;
  try {
    return new URL(origin).host === host;
  } catch {
    return false;
  }
}

export function headerValue(req: IncomingProxyReq, name: string): string | undefined {
  const value = req.headers?.[name];
  if (Array.isArray(value)) return value[0];
  return value;
}

export function guardRpcProxy(proxy: RpcProxy): void {
  proxy.on("proxyReq", (proxyReq: ProxyRequest, req: IncomingProxyReq, res: ProxyResponse) => {
    if (!allowProxyOrigin(headerValue(req, "origin"), headerValue(req, "host"))) {
      res.writeHead?.(403);
      res.end?.();
      proxyReq.destroy?.();
      return;
    }
    proxyReq.setHeader("Origin", backendOrigin);
  });
  proxy.on("proxyReqWs", (proxyReq: ProxyRequest, req: IncomingProxyReq, socket: UpgradeSocket) => {
    if (!allowProxyOrigin(headerValue(req, "origin"), headerValue(req, "host"))) {
      socket.write("HTTP/1.1 403 Forbidden\r\nConnection: close\r\n\r\n");
      socket.destroy();
      proxyReq.destroy?.();
      return;
    }
    proxyReq.setHeader("Origin", backendOrigin);
  });
}
