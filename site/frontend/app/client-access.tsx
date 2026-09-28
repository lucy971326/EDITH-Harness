"use client";

import { useRef } from "react";

const sourceUrl = "https://github.com/lucy971326/EDITH-Harness#readme";

export function ClientAccess({ className }: { className: string }) {
  const dialogRef = useRef<HTMLDialogElement>(null);

  return (
    <>
      <button className={className} type="button" onClick={() => dialogRef.current?.showModal()}>
        获取客户端
      </button>
      <dialog
        className="availability-dialog"
        ref={dialogRef}
        onClick={(event) => {
          if (event.target === dialogRef.current) dialogRef.current.close();
        }}
      >
        <div className="availability-content">
          <p className="section-kicker">EDITH / CLIENT</p>
          <h2>安装包正在准备中。</h2>
          <p>EDITH 目前可以从源码运行。正式安装包发布前，可以先查看项目和启动说明。</p>
          <div className="availability-actions">
            <a className="site-button site-button-primary" href={sourceUrl} target="_blank" rel="noreferrer">
              查看源码与运行说明
            </a>
            <button className="site-button site-button-quiet" type="button" onClick={() => dialogRef.current?.close()}>
              关闭
            </button>
          </div>
        </div>
      </dialog>
    </>
  );
}
