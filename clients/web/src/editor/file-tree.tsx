import { useEffect, useState } from "react";
import type { FileEntry } from "../../../contracts/appserver";
import type { RPCClient } from "../client/rpc";
import { ChevronRight, FileText, Folder, FolderOpen } from "../icons";
import { fileName, joinPath } from "./files";

export function FileTree({
  workspace,
  client,
  revision,
  activePath,
  onOpenFile,
  onOpenDirectory,
}: {
  workspace: string;
  client: RPCClient | null;
  revision: number;
  activePath?: string;
  onOpenFile: (path: string) => void;
  onOpenDirectory: (path: string) => void;
}) {
  return (
    <nav className="file-tree" aria-label="项目文件">
      <DirectoryNode
        path={workspace}
        name={fileName(workspace)}
        client={client}
        revision={revision}
        activePath={activePath}
        depth={0}
        initialOpen
        onOpenFile={onOpenFile}
        onOpenDirectory={onOpenDirectory}
      />
    </nav>
  );
}

function DirectoryNode({
  path,
  name,
  client,
  revision,
  activePath,
  depth,
  initialOpen = false,
  onOpenFile,
  onOpenDirectory,
}: {
  path: string;
  name: string;
  client: RPCClient | null;
  revision: number;
  activePath?: string;
  depth: number;
  initialOpen?: boolean;
  onOpenFile: (path: string) => void;
  onOpenDirectory: (path: string) => void;
}) {
  const [open, setOpen] = useState(initialOpen);
  const [entries, setEntries] = useState<FileEntry[] | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!open || !client?.connected) return;
    let current = true;
    setError("");
    void client
      .readDirectory(path)
      .then((result) => {
        if (!current) return;
        setEntries(
          [...result.entries].sort(
            (left, right) =>
              Number(right.isDirectory) - Number(left.isDirectory) ||
              left.fileName.localeCompare(right.fileName),
          ),
        );
        onOpenDirectory(path);
      })
      .catch((reason) => {
        if (current)
          setError(reason instanceof Error ? reason.message : "目录读取失败");
      });
    return () => {
      current = false;
    };
  }, [client, open, path, revision, onOpenDirectory]);

  function toggle() {
    setOpen((value) => !value);
  }

  async function openEntry(entry: FileEntry, childPath: string) {
    if (entry.isFile) {
      onOpenFile(childPath);
      return;
    }
    if (!client?.connected) return;
    try {
      const metadata = await client.getMetadata(childPath);
      if (metadata.isFile) onOpenFile(childPath);
    } catch {
      /* 类型不明或目标消失时，等待下一次目录通知刷新。 */
    }
  }

  return (
    <div className="file-tree-node">
      <button
        className="file-tree-row"
        style={{ paddingLeft: `${8 + depth * 14}px` }}
        title={path}
        data-file-path={path}
        onClick={toggle}
      >
        <ChevronRight className="file-tree-chevron" data-open={open} />
        {open ? <FolderOpen /> : <Folder />}
        <span>{name}</span>
      </button>
      {open && (
        <div role="group">
          {!entries && !error && (
            <div
              className="file-tree-note"
              style={{ paddingLeft: `${28 + depth * 14}px` }}
            >
              正在读取…
            </div>
          )}
          {error && (
            <div
              className="file-tree-note file-tree-error"
              style={{ paddingLeft: `${28 + depth * 14}px` }}
            >
              {error}
            </div>
          )}
          {entries?.map((entry) => {
            const childPath = joinPath(path, entry.fileName);
            if (entry.isDirectory)
              return (
                <DirectoryNode
                  key={childPath}
                  path={childPath}
                  name={entry.fileName}
                  client={client}
                  revision={revision}
                  activePath={activePath}
                  depth={depth + 1}
                  onOpenFile={onOpenFile}
                  onOpenDirectory={onOpenDirectory}
                />
              );
            return (
              <button
                key={childPath}
                className="file-tree-row"
                data-active={childPath === activePath}
                style={{ paddingLeft: `${26 + depth * 14}px` }}
                title={childPath}
                data-file-path={childPath}
                onClick={() => void openEntry(entry, childPath)}
              >
                <FileText />
                <span>{entry.fileName}</span>
              </button>
            );
          })}
          {entries?.length === 0 && (
            <div
              className="file-tree-note"
              style={{ paddingLeft: `${28 + depth * 14}px` }}
            >
              空目录
            </div>
          )}
        </div>
      )}
    </div>
  );
}
