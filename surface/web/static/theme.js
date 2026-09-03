(() => {
  const storageKey = "harness.theme";
  const values = new Set(["system", "light", "dark"]);
  const root = document.documentElement;

  const readTheme = () => {
    try {
      const value = window.localStorage.getItem(storageKey);
      return values.has(value) ? value : "system";
    } catch (_) {
      return "system";
    }
  };

  const setTheme = (value) => {
    const theme = values.has(value) ? value : "system";
    root.dataset.theme = theme;
    try {
      window.localStorage.setItem(storageKey, theme);
    } catch (_) {
      // 隐私模式下仍保持本次页面的主题效果。
    }
  };

  const syncSelect = () => {
    const select = document.querySelector("#theme-select");
    if (select) {
      select.value = root.dataset.theme || "system";
    }
  };

  root.dataset.theme = readTheme();
  document.addEventListener("change", (event) => {
    if (event.target instanceof HTMLSelectElement && event.target.id === "theme-select") {
      setTheme(event.target.value);
    }
  });
  document.addEventListener("DOMContentLoaded", syncSelect);
  document.addEventListener("htmx:afterSwap", syncSelect);
})();
