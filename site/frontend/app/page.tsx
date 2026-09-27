import { Show, SignUpButton } from "@clerk/nextjs";
import Link from "next/link";

export default function Home() {
  return (
    <main className="mx-auto flex w-full max-w-6xl flex-1 flex-col justify-center px-6 py-20 sm:py-28">
      <div className="max-w-2xl">
        <p className="mb-5 inline-flex rounded-full border border-(--border) bg-(--surface) px-3 py-1 text-xs font-medium text-(--muted)">
          EDITH · 网站基础建设中
        </p>
        <h1 className="text-4xl font-semibold leading-tight tracking-tight sm:text-6xl">
          在自己的电脑上，<br />
          让 AI 帮你完成工作。
        </h1>
        <p className="mt-6 max-w-xl text-base leading-8 text-(--muted) sm:text-lg">
          EDITH 已支持本机 Web 与 Desktop。这个网站正在搭建账号与设备连接的基础；远程控制功能还在开发中。
        </p>
        <div className="mt-9 flex flex-wrap items-center gap-3">
          <Show when="signed-out">
            <SignUpButton>
              <button className="rounded-xl bg-(--foreground) px-5 py-3 text-sm font-medium text-(--background) transition hover:opacity-85" type="button">
                创建账号
              </button>
            </SignUpButton>
          </Show>
          <Show when="signed-in">
            <Link className="rounded-xl bg-(--foreground) px-5 py-3 text-sm font-medium text-(--background) transition hover:opacity-85" href="/account">
              进入控制台
            </Link>
          </Show>
          <span className="text-sm text-(--muted)">远程控制 · 开发中</span>
        </div>
      </div>
    </main>
  );
}
