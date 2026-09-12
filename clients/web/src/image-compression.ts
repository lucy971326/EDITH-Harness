import type { Attachment } from "./composer";

const maxDimension = 2048;
const maxBytes = 2 * 1024 * 1024;
const maxSourceBytes = 10 * 1024 * 1024;
const maxSourcePixels = 50_000_000;
const accepted = new Set([
  "image/png",
  "image/jpeg",
  "image/webp",
  "image/gif",
]);

export async function compressImage(file: File): Promise<Attachment> {
  if (!accepted.has(file.type)) {
    throw new Error("请选择 PNG、JPEG、WebP 或 GIF 图片");
  }
  if (file.size > maxSourceBytes) {
    throw new Error("原图请不超过 10 MB");
  }

  const bitmap = await createImageBitmap(file);
  try {
    if (bitmap.width * bitmap.height > maxSourcePixels) {
      throw new Error("图片像素过大，请先缩小图片");
    }
    let scale = Math.min(
      1,
      maxDimension / Math.max(bitmap.width, bitmap.height),
    );
    let quality = 0.86;
    for (let attempt = 0; attempt < 10; attempt++) {
      const width = Math.max(1, Math.round(bitmap.width * scale));
      const height = Math.max(1, Math.round(bitmap.height * scale));
      const canvas = document.createElement("canvas");
      canvas.width = width;
      canvas.height = height;
      const context = canvas.getContext("2d");
      if (!context) throw new Error("浏览器无法处理这张图片");
      context.drawImage(bitmap, 0, 0, width, height);
      const blob = await canvasBlob(canvas, quality);
      if (blob.size <= maxBytes) {
        return {
          id: crypto.randomUUID(),
          name: file.name,
          url: URL.createObjectURL(blob),
          mime: "image/webp",
          data: await blobBase64(blob),
        };
      }
      if (quality > 0.55) quality -= 0.1;
      else scale *= 0.82;
    }
  } finally {
    bitmap.close();
  }
  throw new Error("压缩后仍超过 2 MB，请换一张更小的图片");
}

function canvasBlob(canvas: HTMLCanvasElement, quality: number): Promise<Blob> {
  return new Promise((resolve, reject) => {
    canvas.toBlob(
      (blob) => {
        if (!blob || blob.type !== "image/webp") {
          reject(new Error("当前浏览器不支持 WebP 压缩"));
          return;
        }
        resolve(blob);
      },
      "image/webp",
      quality,
    );
  });
}

function blobBase64(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onerror = () => reject(new Error("无法读取压缩后的图片"));
    reader.onload = () => {
      const value = String(reader.result);
      const comma = value.indexOf(",");
      if (comma < 0) {
        reject(new Error("无法编码压缩后的图片"));
        return;
      }
      resolve(value.slice(comma + 1));
    };
    reader.readAsDataURL(blob);
  });
}
