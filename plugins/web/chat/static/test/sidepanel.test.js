const test = require("node:test");
const assert = require("node:assert/strict");
const panel = require("../sidepanel.js");

test("不同实例新开，相同实例聚焦", () => {
  const state = panel.createState();
  panel.openTab(state, { type: "test", instanceKey: "one", title: "一" });
  panel.openTab(state, { type: "test", instanceKey: "two", title: "二" });
  panel.openTab(state, { type: "test", instanceKey: "one", title: "一" });
  assert.equal(state.tabs.length, 2);
  assert.equal(state.activeKey, "test:one");
});

test("关闭选中 Tab 后回退到相邻 Tab", () => {
  const state = panel.createState();
  panel.openTab(state, { type: "test", instanceKey: "one", title: "一" });
  panel.openTab(state, { type: "test", instanceKey: "two", title: "二" });
  panel.closeTab(state, "test:two");
  assert.equal(state.activeKey, "test:one");
  assert.equal(state.tabs.length, 1);
});
