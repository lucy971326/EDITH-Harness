import { ClerkProvider, Show, SignInButton, SignUpButton, UserButton } from "@clerk/nextjs";
import type { Metadata } from "next";
import Image from "next/image";
import Link from "next/link";
import { Geist, Geist_Mono } from "next/font/google";
import "./globals.css";

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

export const metadata: Metadata = {
  title: "EDITH",
  description: "让你的 AI 工作留在自己的电脑上。",
};

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html
      lang="zh-CN"
      className={`${geistSans.variable} ${geistMono.variable} h-full antialiased`}
    >
      <body className="flex min-h-full flex-col">
        <ClerkProvider>
          <header className="border-b border-(--border)">
            <div className="mx-auto flex h-16 w-full max-w-6xl items-center justify-between gap-4 px-6">
              <Link className="inline-flex items-center gap-2 text-base font-semibold tracking-[0.06em]" href="/">
                <Image src="/icon.svg" width={28} height={28} alt="" />
                EDITH
              </Link>
              <nav aria-label="账号导航" className="flex items-center gap-3 text-sm font-medium">
                <Show when="signed-out">
                  <SignInButton>
                    <button className="rounded-lg px-3 py-2 text-(--muted) transition hover:text-(--foreground)" type="button">
                      登录
                    </button>
                  </SignInButton>
                  <SignUpButton>
                    <button className="rounded-lg bg-(--foreground) px-4 py-2 text-(--background) transition hover:opacity-85" type="button">
                      注册
                    </button>
                  </SignUpButton>
                </Show>
                <Show when="signed-in">
                  <Link className="rounded-lg px-3 py-2 transition hover:bg-(--surface)" href="/account">
                    控制台
                  </Link>
                  <UserButton />
                </Show>
              </nav>
            </div>
          </header>
          {children}
        </ClerkProvider>
      </body>
    </html>
  );
}
