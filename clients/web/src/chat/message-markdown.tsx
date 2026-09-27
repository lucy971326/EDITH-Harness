import Markdown, { defaultUrlTransform } from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeSanitize, { defaultSchema } from "rehype-sanitize";
import type { Root, RootContent } from "hast";
import { parseFileLink, type FileLocation } from "../editor/links";
import { skillParts, SkillTag } from "./skill-mentions";

// URL 已由 react-markdown 过滤；这里仅放行 Windows 盘符，其他 HTML 仍按默认规则清洗。
const markdownSchema = {
  ...defaultSchema,
  protocols: { ...defaultSchema.protocols, href: [] },
};

// 在清洗 HTML 之后只装饰用户正文的纯文本节点，代码与链接保持原样。
function skillTokens(options: { names: ReadonlySet<string> }) {
  return (root: Root) => {
    function visit(node: Root | RootContent) {
      if (node.type === "element" && ["pre", "code", "a"].includes(node.tagName)) return;
      if (!("children" in node)) return;
      const children = node.children as RootContent[];
      for (let index = 0; index < children.length; index++) {
        const child = children[index];
        if (child.type !== "text") {
          visit(child);
          continue;
        }
        const parts = skillParts(child.value, options.names);
        if (!parts.some((part) => part.kind === "skill")) continue;
        const replacement: RootContent[] = parts.map((part) => part.kind === "text"
          ? { type: "text", value: part.value }
          : { type: "element", tagName: "span", properties: { dataSkillName: part.name },
              children: [{ type: "text", value: part.name }] });
        children.splice(index, 1, ...replacement);
        index += replacement.length - 1;
      }
    }
    visit(root);
  };
}

// 不执行原始 HTML；清洗渲染树。图片另走附件能力，不自动请求正文里的远程图片。
export function MessageMarkdown({
  text,
  workspace,
  onOpenFile,
  skillNames = [],
}: {
  text: string;
  workspace?: string | null;
  onOpenFile?: (location: FileLocation) => void;
  skillNames?: string[];
}) {
  const names = new Set(skillNames);
  return (
    <div className="message-markdown">
      <Markdown
        remarkPlugins={[remarkGfm]}
        rehypePlugins={[[rehypeSanitize, markdownSchema], [skillTokens, { names }]]}
        skipHtml
        urlTransform={(url) =>
          /^[a-z]:[\\/]/i.test(url) ? url : defaultUrlTransform(url)
        }
        components={{
          span: ({ node, children, ...props }) => {
            const name = node?.properties?.dataSkillName;
            return typeof name === "string" ? <SkillTag name={name} />
              : <span {...props}>{children}</span>;
          },
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
