import { ClerkProvider, Show, SignInButton, UserButton } from "@clerk/nextjs";
import type { Metadata } from "next";
import Image from "next/image";
import Link from "next/link";
import { ClientAccess } from "./client-access";
import "./globals.css";

export const metadata: Metadata = {
  title: "EDITH — 在自己的电脑上，把工作做完",
  description: "EDITH 是运行在你自己电脑上的 AI 工作助手，帮你读文件、写代码、调用工具。",
};

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html lang="zh-CN" className="h-full antialiased">
      <body className="flex min-h-full flex-col">
        <ClerkProvider>
          <header className="site-header">
            <div className="site-container header-inner">
              <Link className="site-brand" href="/" aria-label="EDITH 首页">
                <Image src="/icon.svg" width={37} height={37} alt="" />
                EDITH
              </Link>
              <nav aria-label="网站导航" className="site-nav">
                <Link href="/#product">产品</Link>
                <a href="https://github.com/lucy971326/EDITH-Harness#readme" target="_blank" rel="noreferrer">
                  文档
                </a>
              </nav>
              <nav aria-label="账号导航" className="header-actions">
                <Show when="signed-out">
                  <SignInButton>
                    <button className="header-text-action" type="button">
                      登录
                    </button>
                  </SignInButton>
                </Show>
                <Show when="signed-in">
                  <Link className="header-text-action" href="/account">
                    控制台
                  </Link>
                  <UserButton />
                </Show>
                <ClientAccess className="site-button site-button-primary header-download" />
              </nav>
            </div>
          </header>
          {children}
        </ClerkProvider>
      </body>
    </html>
  );
}
