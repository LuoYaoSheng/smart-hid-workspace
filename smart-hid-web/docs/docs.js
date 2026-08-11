/* Smart HID — 文档页 scrollspy：随滚动高亮侧边目录 */
(function () {
  "use strict";
  var toc = document.getElementById("toc");
  if (!toc) return;
  var links = Array.prototype.slice.call(toc.querySelectorAll("a"));
  var targets = links.map(function (a) {
    var id = a.getAttribute("href").slice(1);
    return { link: a, el: document.getElementById(id) };
  }).filter(function (t) { return t.el; });

  if (!("IntersectionObserver" in window)) return;

  var current = null;
  var io = new IntersectionObserver(function (entries) {
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
  if (targets[0]) targets[0].link.classList.add("active");
})();
