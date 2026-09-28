import Image from "next/image";
import Link from "next/link";
import { ClientAccess } from "./client-access";

export default function Home() {
  return (
    <main id="main-content">
      <section className="hero" aria-labelledby="hero-title">
        <div className="site-container hero-layout">
          <div className="hero-copy">
            <h1 id="hero-title">
              让 AI<br />
              在你的电脑上，<br />
              把工作做完<span className="orange-period">。</span>
            </h1>
            <p className="hero-description">
              EDITH 是运行在你自己电脑上的 AI 工作助手。读文件、写代码、调用工具，帮你把想法变成结果。
            </p>
            <div className="hero-actions">
              <ClientAccess className="site-button site-button-primary site-button-large" />
              <Link className="site-button site-button-outline site-button-large" href="#product">
                了解 EDITH
              </Link>
            </div>
            <p className="hero-footnote">在你自己的电脑上工作，由你掌控项目与工具。</p>
          </div>
          <div className="hero-visual">
            <Image
              className="hero-image"
              src="/images/edith-workspace.png"
              alt="EDITH 工作区界面示意：左侧是项目会话，右侧是与 AI 协作的内容区"
              width={1470}
              height={1070}
              sizes="(max-width: 900px) 100vw, 62vw"
              priority
            />
          </div>
        </div>
      </section>

      <section className="product-story" id="product" aria-labelledby="product-title">
        <div className="site-container story-layout">
          <div className="story-copy">
            <p className="section-kicker">本地的工作方式</p>
            <h2 id="product-title">
              更专注，<br />更安心<span className="orange-period">。</span>
            </h2>
            <p>
              EDITH 在你的电脑上运行，理解眼前的项目和上下文，按你的授权使用工具，在熟悉的工作环境中把事情一步步做完。
            </p>
            <a className="story-link" href="https://github.com/lucy971326/EDITH-Harness#readme" target="_blank" rel="noreferrer">
              查看项目与运行说明
            </a>
          </div>
          <Image
            className="story-image"
            src="/images/workspace-light.png"
            alt="阳光照进安静的工作空间"
            width={1536}
            height={1024}
            sizes="(max-width: 900px) 100vw, 55vw"
          />
        </div>
      </section>

      <footer className="site-footer">
        <div className="site-container footer-inner">
          <span>EDITH · 在自己的电脑上，专心把工作做好。</span>
          <a href="https://github.com/lucy971326/EDITH-Harness" target="_blank" rel="noreferrer">GitHub</a>
        </div>
      </footer>
    </main>
  );
}
