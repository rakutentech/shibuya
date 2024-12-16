// Populate the sidebar
//
// This is a script, and not included directly in the page, to control the total size of the book.
// The TOC contains an entry for each page, so if each page includes a copy of the TOC,
// the total size of the page becomes O(n**2).
class MDBookSidebarScrollbox extends HTMLElement {
    constructor() {
        super();
    }
    connectedCallback() {
        this.innerHTML = '<ol class="chapter"><li class="chapter-item expanded affix "><a href="introduction.html">Introduction</a></li><li class="chapter-item expanded "><a href="ops/ops_intro.html"><strong aria-hidden="true">1.</strong> How to run Shibuya</a></li><li><ol class="section"><li class="chapter-item expanded "><a href="ops/requirements.html"><strong aria-hidden="true">1.1.</strong> Basic Requirements</a></li><li class="chapter-item expanded "><a href="ops/authentication.html"><strong aria-hidden="true">1.2.</strong> Authentication model and multi tenancy</a></li><li class="chapter-item expanded "><a href="ops/docker.html"><strong aria-hidden="true">1.3.</strong> Dockerfile</a></li><li class="chapter-item expanded "><a href="ops/config.html"><strong aria-hidden="true">1.4.</strong> Configuration explanation</a></li><li class="chapter-item expanded "><a href="ops/object_storage.html"><strong aria-hidden="true">1.5.</strong> Object Storage</a></li></ol></li><li class="chapter-item expanded "><a href="user/user_guide_intro.html"><strong aria-hidden="true">2.</strong> How to use Shibuya</a></li><li><ol class="section"><li class="chapter-item expanded "><a href="user/concept.html"><strong aria-hidden="true">2.1.</strong> Basic Concepts</a></li><li class="chapter-item expanded "><a href="user/faq.html"><strong aria-hidden="true">2.2.</strong> FAQ</a></li></ol></li><li class="chapter-item expanded "><a href="dev/intro.html"><strong aria-hidden="true">3.</strong> Shibuya developers</a></li></ol>';
        // Set the current, active page, and reveal it if it's hidden
        let current_page = document.location.href.toString();
        if (current_page.endsWith("/")) {
            current_page += "index.html";
        }
        var links = Array.prototype.slice.call(this.querySelectorAll("a"));
        var l = links.length;
        for (var i = 0; i < l; ++i) {
            var link = links[i];
            var href = link.getAttribute("href");
            if (href && !href.startsWith("#") && !/^(?:[a-z+]+:)?\/\//.test(href)) {
                link.href = path_to_root + href;
            }
            // The "index" page is supposed to alias the first chapter in the book.
            if (link.href === current_page || (i === 0 && path_to_root === "" && current_page.endsWith("/index.html"))) {
                link.classList.add("active");
                var parent = link.parentElement;
                if (parent && parent.classList.contains("chapter-item")) {
                    parent.classList.add("expanded");
                }
                while (parent) {
                    if (parent.tagName === "LI" && parent.previousElementSibling) {
                        if (parent.previousElementSibling.classList.contains("chapter-item")) {
                            parent.previousElementSibling.classList.add("expanded");
                        }
                    }
                    parent = parent.parentElement;
                }
            }
        }
        // Track and set sidebar scroll position
        this.addEventListener('click', function(e) {
            if (e.target.tagName === 'A') {
                sessionStorage.setItem('sidebar-scroll', this.scrollTop);
            }
        }, { passive: true });
        var sidebarScrollTop = sessionStorage.getItem('sidebar-scroll');
        sessionStorage.removeItem('sidebar-scroll');
        if (sidebarScrollTop) {
            // preserve sidebar scroll position when navigating via links within sidebar
            this.scrollTop = sidebarScrollTop;
        } else {
            // scroll sidebar to current active section when navigating via "next/previous chapter" buttons
            var activeSection = document.querySelector('#sidebar .active');
            if (activeSection) {
                activeSection.scrollIntoView({ block: 'center' });
            }
        }
        // Toggle buttons
        var sidebarAnchorToggles = document.querySelectorAll('#sidebar a.toggle');
        function toggleSection(ev) {
            ev.currentTarget.parentElement.classList.toggle('expanded');
        }
        Array.from(sidebarAnchorToggles).forEach(function (el) {
            el.addEventListener('click', toggleSection);
        });
    }
}
window.customElements.define("mdbook-sidebar-scrollbox", MDBookSidebarScrollbox);
