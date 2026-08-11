/*!
 * Smart HID — 通用 Markdown 文档加载器（零构建）
 * 配合 docs.css / docs.js 使用。
 *
 * 用法：在 <main class="docs-main"> 上声明 data-md="文件名.md"
 *   <main class="docs-main" data-md="01_PRODUCT_PRD.md" data-strip-h1="1">
 *     <span class="kicker">...</span><h1>页面标题</h1><p class="docs-lead">导语</p>
 *     <div class="md-body" id="md-body"><p style="color:#94a3b8">加载中…</p></div>
 *   </main>
 *
 * 行为：fetch 同目录 .md → marked 渲染进 #md-body → 给正文 h2/h3 生成 id
 *       → 在 #toc 侧栏生成目录链接 → 调用 window.initDocsScrollspy() 重建高亮。
 *       data-strip-h1="1" 时去掉 md 首个一级标题（避免与页面手写 h1 重复）。
 *       file:// 下 fetch 被浏览器拦截，给出友好提示。
 */
(function () {
  "use strict";

  function slug(text) {
    return "sec-" + String(text)
      .trim().toLowerCase()
      .replace(/[^\w\u4e00-\u9fa5]+/g, "-")
      .replace(/^-+|-+$/g, "");
  }

  function loadMarked() {
    return new Promise(function (resolve, reject) {
      if (window.marked && typeof window.marked.parse === "function") {
        return resolve(window.marked);
      }
      var s = document.createElement("script");
      s.src = "https://cdn.jsdelivr.net/npm/marked@4.3.0/marked.min.js";
      s.async = true;
      s.onload = function () { resolve(window.marked); };
      s.onerror = function () { reject(new Error("marked 库加载失败（检查网络/CDN）")); };
      document.head.appendChild(s);
    });
  }

  function init() {
    var main = document.querySelector("main[data-md]");
    if (!main) return;
    var mdUrl = main.getAttribute("data-md");
    var body = document.getElementById("md-body") || main;
    var stripH1 = main.getAttribute("data-strip-h1") === "1";

    loadMarked().then(function (marked) {
      return fetch(mdUrl, { cache: "no-cache" }).then(function (r) {
        if (!r.ok) throw new Error("HTTP " + r.status);
        return r.text();
      }).then(function (text) {
        if (stripH1) text = text.replace(/^#\s+[^\n]*\n+/, "");
        body.innerHTML = marked.parse(text);

        // 给正文 h2/h3 加 id（去重）
        var heads = body.querySelectorAll("h2, h3");
        var used = {};
        heads.forEach(function (h) {
          var id = slug(h.textContent);
          while (used[id]) { id += "-x"; }
          used[id] = 1;
          h.id = id;
        });

        // 生成侧边目录（保留 .toc-title）
        var toc = document.getElementById("toc");
        if (toc) {
          var title = toc.querySelector(".toc-title");
          toc.innerHTML = "";
          if (title) toc.appendChild(title);
          heads.forEach(function (h) {
            var a = document.createElement("a");
            a.href = "#" + h.id;
            a.textContent = h.textContent;
            if (h.tagName === "H3") a.style.paddingLeft = "20px";
            toc.appendChild(a);
          });
        }

        // 通知 docs.js 重建 scrollspy
        if (typeof window.initDocsScrollspy === "function") {
          window.initDocsScrollspy();
        }
      });
    }).catch(function (err) {
      body.innerHTML =
        '<div class="callout warn"><strong>文档加载失败：</strong>' + err.message +
        "。请通过本地静态服务器访问（如 <code>python3 -m http.server</code>），" +
        "浏览器在 <code>file://</code> 协议下会拦截 fetch 请求。</div>";
    });
  }

  if (document.readyState !== "loading") init();
  else document.addEventListener("DOMContentLoaded", init);
})();
