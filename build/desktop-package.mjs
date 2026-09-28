import { existsSync, mkdirSync, readFileSync, renameSync } from "node:fs";
import { spawnSync } from "node:child_process";
import { resolve } from "node:path";

// 只编排构建工具；应用运行时不依赖此脚本或 NSIS。
function run(command, args, capture = false) {
  const result = spawnSync(command, args, { stdio: capture ? "pipe" : "inherit", encoding: "utf8" });
  if (result.error || result.status !== 0) {
    throw new Error(`${command} 执行失败：${result.error?.message ?? result.stderr ?? result.status}`);
  }
  return result.stdout?.trim();
}

try {
  const expected = readFileSync("go.mod", "utf8").match(/github\.com\/wailsapp\/wails\/v3\s+(\S+)/)?.[1];
  const runtime = JSON.parse(readFileSync("clients/package.json", "utf8")).dependencies["@wailsio/runtime"];
  const cli = spawnSync("wails3", ["version"], { encoding: "utf8" });
  if (!expected || cli.status !== 0 || `${cli.stdout ?? ""}${cli.stderr ?? ""}`.trim() !== expected || `v${runtime}` !== expected) {
    throw new Error(`Wails 版本须统一为 ${expected}。运行 go install github.com/wailsapp/wails/v3/cmd/wails3@${expected}，并对齐 @wailsio/runtime。`);
  }
  if (!["win32", "darwin"].includes(process.platform)) {
    throw new Error("Desktop 打包仅支持 Windows 和 macOS 本机构建。");
  }
  const environment = JSON.parse(run("go", ["env", "-json", "GOARCH", "GOHOSTARCH", "GOOS", "GOHOSTOS"], true));
  const architecture = environment.GOARCH;
  if (architecture !== environment.GOHOSTARCH || environment.GOOS !== environment.GOHOSTOS || !["amd64", "arm64"].includes(architecture)) {
    throw new Error("请使用本机 amd64 或 arm64 架构打包，不支持交叉编译。");
  }
  const version = readFileSync("build/version.txt", "utf8").trim();
  if (!/^\d+\.\d+\.\d+$/.test(version) || version.split(".").some((part) => Number(part) > 65535)) {
    throw new Error("build/version.txt 须为三段数字版本，各段不超过 65535。");
  }

  let nsis;
  if (process.platform === "win32") {
    const candidates = ["makensis.exe", ...[process.env["ProgramFiles(x86)"], process.env.ProgramFiles,
      process.env.LOCALAPPDATA && resolve(process.env.LOCALAPPDATA, "Programs")]
      .filter(Boolean).map((directory) => resolve(directory, "NSIS/makensis.exe"))];
    nsis = candidates.find((command) => spawnSync(command, ["/VERSION"], { encoding: "utf8" }).status === 0);
    if (!nsis) throw new Error("缺少 NSIS。请运行 winget install --id NSIS.NSIS --exact，再重新打包。");
  }
  if (process.argv[2] === "--check") process.exit(0);

  if (process.platform === "win32") {
    if (!existsSync(".build/EDITH.exe")) throw new Error("缺少 .build/EDITH.exe，请使用 make desktop-package。");
    mkdirSync(".build/temp", { recursive: true });
    run("wails3", ["generate", "webview2bootstrapper", "-dir", ".build/temp"]);
    run(nsis, ["/V2", "/INPUTCHARSET", "UTF8", `/DVERSION=${version}`, `/DARCH=${architecture}`,
      `/DOUTPUT=${resolve(`.build/EDITH-${version}-windows-${architecture}-setup.exe`)}`,
      `/DBINARY=${resolve(".build/EDITH.exe")}`, `/DBOOTSTRAPPER=${resolve(".build/temp/MicrosoftEdgeWebview2Setup.exe")}`,
      resolve("build/windows/installer.nsi")]);
  } else {
    run("wails3", ["tool", "package", "--format", "dmg", "--name", "EDITH", "--out", ".build",
      "--volume-icon", "build/macos/icon.icns", "--file-icon", "build/macos/icon.icns"]);
    renameSync(".build/EDITH.dmg", `.build/EDITH-${version}-macos-${architecture}.dmg`);
  }
} catch (error) {
  console.error(error.message);
  process.exit(1);
}
