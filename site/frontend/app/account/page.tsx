import { auth } from "@clerk/nextjs/server";

export default async function AccountPage() {
  await auth.protect();

  return (
    <main className="mx-auto w-full max-w-6xl flex-1 px-6 py-16">
      <p className="text-sm font-medium text-(--muted)">个人控制台</p>
      <h1 className="mt-3 text-3xl font-semibold tracking-tight">欢迎来到 EDITH</h1>
      <p className="mt-4 max-w-xl leading-7 text-(--muted)">
        账号已就绪。设备连接与远程控制正在开发中，当前没有可管理的设备。
      </p>
      <section className="mt-10 rounded-2xl border border-(--border) bg-(--surface) p-6">
        <h2 className="font-semibold">远程控制</h2>
        <p className="mt-2 text-sm text-(--muted)">开发中</p>
      </section>
    </main>
  );
}
