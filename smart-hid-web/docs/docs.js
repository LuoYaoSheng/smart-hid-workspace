/* Smart HID — 文档页 scrollspy：随滚动高亮侧边目录
   支持运行时重新初始化：markdown-loader 在 md 注入后会调用 initDocsScrollspy()。 */
(function () {
  "use strict";
  var io = null;
  var current = null;

  window.initDocsScrollspy = function () {
    if (io) { io.disconnect(); io = null; }
    current = null;
    var toc = document.getElementById("toc");
    if (!toc) return;
    var links = Array.prototype.slice.call(toc.querySelectorAll("a"));
    links.forEach(function (a) { a.classList.remove("active"); });

    var targets = links.map(function (a) {
      var id = a.getAttribute("href").slice(1);
      return { link: a, el: document.getElementById(id) };
    }).filter(function (t) { return t.el; });

    if (!targets.length || !("IntersectionObserver" in window)) return;

    io = new IntersectionObserver(function (entries) {
      entries.forEach(function (e) {
        var t = targets.find(function (x) { return x.el === e.target; });
        if (!t) return;
        if (e.isIntersecting) {
          if (current) current.link.classList.remove("active");
          t.link.classList.add("active");
          current = t;
        }
      });
    }, { rootMargin: "-80px 0px -70% 0px", threshold: 0 });

    targets.forEach(function (t) { io.observe(t.el); });
    targets[0].link.classList.add("active");
    current = targets[0];
  };

  if (document.readyState !== "loading") window.initDocsScrollspy();
  else document.addEventListener("DOMContentLoaded", window.initDocsScrollspy);
})();
