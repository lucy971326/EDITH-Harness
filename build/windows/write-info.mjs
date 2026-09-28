import { mkdirSync, readFileSync, writeFileSync } from "node:fs";

const version = readFileSync(new URL("../version.txt", import.meta.url), "utf8").trim();
const info = {
  fixed: { file_version: `${version}.0`, product_version: `${version}.0` },
  info: { "0409": { FileVersion: version, ProductVersion: version, ProductName: "EDITH", FileDescription: "EDITH" } },
};

mkdirSync(new URL("../../.build/temp/", import.meta.url), { recursive: true });
writeFileSync(new URL("../../.build/temp/windows-info.json", import.meta.url), JSON.stringify(info));
