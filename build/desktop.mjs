import { copyFileSync, mkdirSync, readFileSync, renameSync, writeFileSync } from "node:fs";
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
  const mode = process.argv[2];
  if (!["build", "dev", "package"].includes(mode)) {
    throw new Error("用法：node build/desktop.mjs build|dev|package");
  }
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
  if (mode === "package" && process.platform === "win32") {
    const candidates = ["makensis.exe", ...[process.env["ProgramFiles(x86)"], process.env.ProgramFiles,
      process.env.LOCALAPPDATA && resolve(process.env.LOCALAPPDATA, "Programs")]
      .filter(Boolean).map((directory) => resolve(directory, "NSIS/makensis.exe"))];
    nsis = candidates.find((command) => spawnSync(command, ["/VERSION"], { encoding: "utf8" }).status === 0);
    if (!nsis) throw new Error("缺少 NSIS。请运行 winget install --id NSIS.NSIS --exact，再重新打包。");
  }
  // 开发热重启只编译 Go；首次前端准备由 Wails dev 配置执行。
  if (mode !== "dev") run("make", ["web-build"]);
  mkdirSync(".build/temp", { recursive: true });
  if (process.platform === "win32") {
    const info = {
      fixed: { file_version: `${version}.0`, product_version: `${version}.0` },
      info: { "0409": { FileVersion: version, ProductVersion: version, ProductName: "EDITH", FileDescription: "EDITH" } },
    };
    writeFileSync(".build/temp/windows-info.json", JSON.stringify(info));
    run("wails3", ["generate", "syso", "-arch", architecture, "-icon", "build/windows/icon.ico",
      "-manifest", "build/windows/app.manifest", "-info", ".build/temp/windows-info.json",
      "-out", `cmd/harness-desktop/rsrc_windows_${architecture}.syso`]);
  } else {
    // 开发版和正式版共用最低系统版本，避免两处构建参数漂移。
    process.env.MACOSX_DEPLOYMENT_TARGET = "13.0";
    process.env.CGO_CFLAGS = "-mmacosx-version-min=13.0";
    process.env.CGO_LDFLAGS = "-mmacosx-version-min=13.0";
  }
  const binary = `.build/EDITH${process.platform === "win32" ? ".exe" : ""}`;
  const ldflags = `${mode !== "dev" && process.platform === "win32" ? "-H windowsgui " : ""}-X main.version=${version}`;
  run("go", ["build", ...(mode === "dev" ? [] : ["-tags", "production", "-trimpath"]),
    "-ldflags", ldflags, "-o", binary, "./cmd/harness-desktop"]);

  if (process.platform === "darwin" && mode !== "dev") {
    const bundle = ".build/EDITH.app/Contents";
    mkdirSync(`${bundle}/MacOS`, { recursive: true });
    mkdirSync(`${bundle}/Resources`, { recursive: true });
    copyFileSync(binary, `${bundle}/MacOS/EDITH`);
    copyFileSync("build/macos/icon.icns", `${bundle}/Resources/EDITH.icns`);
    const properties = {
      CFBundleIdentifier: "com.edith.harness.desktop",
      CFBundleName: "EDITH",
      CFBundleDisplayName: "EDITH",
      CFBundleExecutable: "EDITH",
      CFBundleIconFile: "EDITH.icns",
      CFBundleShortVersionString: version,
      CFBundleVersion: version,
      CFBundlePackageType: "APPL",
      LSMinimumSystemVersion: "13.0",
    };
    writeFileSync(`${bundle}/Info.plist`, `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
${Object.entries(properties).map(([key, value]) => `  <key>${key}</key><string>${value}</string>`).join("\n")}
</dict></plist>\n`);
    run("codesign", ["--force", "--deep", "--sign", "-", ".build/EDITH.app"]);
  }
  if (mode !== "package") process.exit(0);

  if (process.platform === "win32") {
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
