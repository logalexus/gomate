// Run with: node tests/browser/compact-forms.cjs (requires web dependencies).
// All API requests are mocked; this does not modify the running instance.
const { chromium } = require("../../web/node_modules/playwright"),
  fs = require("fs"),
  assert = require("assert/strict"),
  path = require("path"),
  { spawn } = require("child_process");
const root = path.resolve(__dirname, "../..");
const artifact = (name) => path.join(root, "reports", name);
fs.mkdirSync(path.join(root, "reports"), { recursive: true });
(async () => {
  const base = "http://127.0.0.1:4193",
    server = spawn(
      process.execPath,
      [
        "node_modules/vite/bin/vite.js",
        "--host",
        "127.0.0.1",
        "--port",
        "4193",
        "--strictPort",
      ],
      { cwd: path.join(root, "web"), stdio: "ignore" }
    );
  let browser;
  const report = { passed: false, checks: [], errors: [], writes: [] };
  try {
    for (let i = 0; i < 60; i++) {
      try {
        if ((await fetch(base)).ok) break;
      } catch {}
      await new Promise((r) => setTimeout(r, 150));
    }
    browser = await chromium.launch();
    const page = await browser.newPage({
      viewport: { width: 1366, height: 850 },
      reducedMotion: "reduce",
    });
    page.on("pageerror", (e) => report.errors.push(e.message));
    await page.routeWebSocket("**/api/v2/events", () => {});
    const services = [{ port: 8070, name: "Documents API" }],
      patterns = [
        {
          id: 1,
          name: "Suspicious request with a deliberately long pattern name",
          group: "Malicious inputs",
          value: "FLAG_[a-z0-9]+",
          searchType: "REGEX",
          actionType: "FIND",
          directionType: "BOTH",
          serviceId: 8070,
          color: "#ff7474",
          enabled: true,
        },
        {
          id: 2,
          name: "Noise",
          value: "health",
          searchType: "SUBSTRING",
          actionType: "IGNORE",
          directionType: "INPUT",
          color: "#65c9a5",
          enabled: false,
        },
      ];
    await page.route("**/api/**", async (r) => {
      const req = r.request(),
        p = new URL(req.url()).pathname;
      let data = [];
      if (
        req.method() !== "GET" &&
        !/^\/api\/(stream|packet)\//.test(p) &&
        p != "/api/v2/pattern-test"
      )
        report.writes.push(p);
      if (p === "/api/v2/config") data = { requestTimeoutSeconds: 10 };
      if (p === "/api/v2/snapshot")
        data = { services, patterns, metrics: {}, captureRunning: true };
      if (p === "/api/service/") data = services;
      if (p === "/api/pattern/") data = patterns;
      if (p === "/api/v2/settings")
        data = {
          settings: {
            captureEnabled: true,
            interface: "eth1",
            autoIPs: true,
            localIPs: ["192.168.56.103"],
            packetRetentionSeconds: 3600,
            sessionRetentionSeconds: 21600,
            cleanupIntervalSeconds: 60,
            preserveFavorites: true,
            requestTimeoutSeconds: 10,
          },
          capture: {
            running: true,
            interface: "eth1",
            localIPs: ["192.168.56.103"],
          },
        };
      if (p === "/api/v2/interfaces")
        data = [
          { name: "eth1", addresses: ["192.168.56.103"], supported: true },
        ];
      if (p === "/api/v2/pattern-test")
        data = {
          ignored: false,
          matches: [{ startPosition: 0, endPosition: 5 }],
        };
      await r.fulfill({ json: data });
    });
    await page.goto(base + "/#/");
    const fit = async (id) => {
      const box = await page
        .locator(id)
        .evaluate((e) => ({
          outer: e.scrollWidth,
          viewport: innerWidth,
          body: e.querySelector(".modal-body").scrollWidth,
          client: e.querySelector(".modal-body").clientWidth,
        }));
      assert(box.outer <= box.viewport + 1, JSON.stringify(box));
      assert(box.body <= box.client + 1, JSON.stringify(box));
    };
    const close = async (id) => {
      await page
        .locator(id)
        .getByRole("button", { name: /^(Close|Закрыть)$/ })
        .first()
        .click();
      await page.locator(id).waitFor({ state: "hidden" });
    };
    for (const width of [1366, 375])
      for (const theme of ["dark", "light"]) {
        await page.setViewportSize({ width, height: 850 });
        await page.evaluate((t) => (document.body.dataset.theme = t), theme);
        await page
          .getByRole("button", { name: "Настройки", exact: true })
          .click();
        await page.locator("#capture-interface").waitFor();
        for (const tab of [
          "Захват трафика",
          "Хранение и очистка",
          "Приложение",
          "Отображение",
        ]) {
          await page
            .locator("#settingsModal")
            .getByRole("tab", { name: tab, exact: true })
            .click();
          await fit("#settingsModal");
          if (tab === "Хранение и очистка") {
            assert(
              await page
                .locator("#settingsModal")
                .getByRole("button", { name: "Удалить…", exact: true })
                .isDisabled()
            );
            if (width === 1366 && theme === "dark")
              await page.screenshot({
                path: artifact("compact-settings-dark.jpg"),
                quality: 85,
              });
          }
        }
        await close("#settingsModal");
        await page.getByRole("button", { name: /^Шаблоны/ }).click();
        const manager = page.locator("#patternsManager");
        await manager.locator("tbody tr").first().waitFor();
        await fit("#patternsManager");
        if (width === 375 && theme === "dark")
          await page.screenshot({
            path: artifact("compact-patterns-mobile-dark.jpg"),
            quality: 85,
          });
        await manager
          .getByRole("textbox", { name: "Поиск шаблонов" })
          .fill("absent");
        await manager.locator(".pattern-manager-empty").waitFor();
        await manager
          .getByRole("button", {
            name: "Сбросить фильтры шаблонов",
            exact: true,
          })
          .click();
        await manager.locator("tbody tr").first().waitFor();
        await manager
          .getByRole("button", { name: "+ Новый шаблон", exact: true })
          .click();
        const editor = page.locator("#patternModal");
        await editor.locator("#pattern-name").waitFor();
        await editor
          .getByRole("button", { name: "Текст", exact: true })
          .click();
        await editor
          .getByRole("textbox", { name: "Тестовый фрагмент" })
          .fill("FLAG_a");
        await editor
          .getByRole("button", { name: "Проверить", exact: true })
          .click();
        await editor.getByText("Совпадений: 1", { exact: true }).waitFor();
        await fit("#patternModal");
        await close("#patternModal");
        await manager
          .locator("tbody tr")
          .first()
          .getByRole("button", { name: "Ещё", exact: true })
          .click();
        await page
          .getByRole("menuitem", { name: "В прошлом", exact: true })
          .click();
        await page.locator("#lookback-minutes").waitFor();
        await page.getByRole("button", { name: "1 час", exact: true }).click();
        assert.equal(
          await page.locator("#lookback-minutes").inputValue(),
          "60"
        );
        await fit("#lookBackModal");
        await page
          .locator("#lookBackModal")
          .getByRole("button", { name: "Отмена", exact: true })
          .click();
        await page.locator("#lookBackModal").waitFor({ state: "hidden" });
        await close("#patternsManager");
        report.checks.push({
          width,
          theme,
          settingsTabs: 4,
          editorTest: true,
          lookbackPreset: true,
          filters: true,
          noOverflow: true,
        });
      }
    assert.deepEqual(report.writes, []);
    assert.deepEqual(report.errors, []);
    report.passed = true;
  } finally {
    await browser?.close();
    server.kill();
    fs.writeFileSync(
      artifact("compact-forms-ui.json"),
      JSON.stringify(report, null, 2)
    );
  }
  console.log(report);
})().catch((e) => {
  console.error(e);
  process.exitCode = 1;
});
