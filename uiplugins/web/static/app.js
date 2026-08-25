(() => {
  const storedTheme = localStorage.getItem("edith-theme");
  if (storedTheme === "dark") document.body.dataset.theme = "dark";

  document.addEventListener("click", (event) => {
    if (!event.target.closest("[data-theme-toggle]")) return;
    const next = document.body.dataset.theme === "dark" ? "light" : "dark";
    document.body.dataset.theme = next;
    localStorage.setItem("edith-theme", next);
  });

  document.addEventListener("change", (event) => {
    const targetID = event.target.dataset.modelCopy;
    if (!targetID) return;
    const hidden = document.getElementById(targetID);
    if (hidden) hidden.value = event.target.value;
  });

  const sessionID = document.body.dataset.session;
  const projectID = document.body.dataset.project;
  if (!sessionID || !projectID) return;
  const stream = new EventSource(`/events?session=${encodeURIComponent(sessionID)}`);
  stream.addEventListener("refresh", () => {
    htmx.ajax("GET", `/fragments/chat?project=${encodeURIComponent(projectID)}&session=${encodeURIComponent(sessionID)}`, {target: "#chat-panel", swap: "outerHTML"});
  });
})();
