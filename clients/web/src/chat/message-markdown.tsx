import Markdown, { defaultUrlTransform } from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeSanitize, { defaultSchema } from "rehype-sanitize";
import { parseFileLink, type FileLocation } from "../editor/links";

// URL 已由 react-markdown 过滤；这里仅放行 Windows 盘符，其他 HTML 仍按默认规则清洗。
const markdownSchema = {
  ...defaultSchema,
  protocols: { ...defaultSchema.protocols, href: [] },
};

// 不执行原始 HTML；清洗渲染树。图片另走附件能力，不自动请求正文里的远程图片。
export function MessageMarkdown({
  text,
  workspace,
  onOpenFile,
}: {
  text: string;
  workspace?: string | null;
  onOpenFile?: (location: FileLocation) => void;
}) {
  return (
    <div className="message-markdown">
      <Markdown
        remarkPlugins={[remarkGfm]}
        rehypePlugins={[[rehypeSanitize, markdownSchema]]}
        skipHtml
        urlTransform={(url) =>
          /^[a-z]:[\\/]/i.test(url) ? url : defaultUrlTransform(url)
        }
        components={{
          a: ({ href, children }) => {
            const location = parseFileLink(href, workspace ?? null);
            if (location && onOpenFile)
              return (
                <a
                  href={href}
                  onClick={(event) => {
                    event.preventDefault();
                    onOpenFile(location);
                  }}
                >
                  {children}
                </a>
              );
            return (
              <a href={href} target="_blank" rel="noopener noreferrer">
                {children}
              </a>
            );
          },
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
