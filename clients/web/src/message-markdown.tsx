import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeSanitize from "rehype-sanitize";

// 不执行原始 HTML；清洗渲染树。图片另走附件能力，不自动请求正文里的远程图片。
export function MessageMarkdown({ text }: { text: string }) {
  return (
    <div className="message-markdown">
      <Markdown
        remarkPlugins={[remarkGfm]}
        rehypePlugins={[rehypeSanitize]}
        skipHtml
        components={{
          a: ({ href, children }) => (
            <a href={href} target="_blank" rel="noopener noreferrer">
              {children}
            </a>
          ),
          img: ({ alt }) => (
            <span className="metadata">[图片：{alt || "未展示"}]</span>
          ),
        }}
      >
        {text}
      </Markdown>
    </div>
  );
}
