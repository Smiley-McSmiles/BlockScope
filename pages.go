// Copyright (c) 2022, duggavo
// Copyright (c) 2026, WOOSAH
//
// All rights reserved.
//
// Redistribution and use in source and binary forms, with or without modification, are
// permitted provided that the following conditions are met:
//
// 1. Redistributions of source code must retain the above copyright notice, this list of
//    conditions and the following disclaimer.
//
// 2. Redistributions in binary form must reproduce the above copyright notice, this list
//    of conditions and the following disclaimer in the documentation and/or other
//    materials provided with the distribution.
//
// 3. Neither the name of the copyright holder nor the names of its contributors may be
//    used to endorse or promote products derived from this software without specific
//    prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND ANY
// EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED WARRANTIES OF
// MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL
// THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
// SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO,
// PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
// INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT,
// STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF
// THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

package main

import _ "embed"

var Footer string = `
	<footer class="footer"><a href="https://github.com/Smiley-McSmiles/BlockScope">BlockScope</a> · Your node, Your ledger, No middlemen · self-hosted</footer>
	</div>
	<script>
		(function () {
			var root = document.documentElement;

			document.getElementById('theme-toggle').addEventListener('click', function () {
				var light = root.getAttribute('data-theme') !== 'light';
				if (light) {
					root.setAttribute('data-theme', 'light');
				} else {
					root.removeAttribute('data-theme');
				}
				try { localStorage.setItem('theme', light ? 'light' : 'dark'); } catch (e) {}
			});

			document.addEventListener('keydown', function (e) {
				var tag = (e.target.tagName || '').toLowerCase();
				if (e.key === '/' && tag !== 'input' && tag !== 'textarea') {
					e.preventDefault();
					document.getElementById('search').focus();
				}
			});

			function copyText(text) {
				if (navigator.clipboard && window.isSecureContext) {
					return navigator.clipboard.writeText(text);
				}
				return new Promise(function (resolve, reject) {
					var ta = document.createElement('textarea');
					ta.value = text;
					ta.setAttribute('readonly', '');
					ta.style.position = 'fixed';
					ta.style.opacity = '0';
					document.body.appendChild(ta);
					ta.select();
					var ok = false;
					try { ok = document.execCommand('copy'); } catch (e) {}
					document.body.removeChild(ta);
					ok ? resolve() : reject();
				});
			}

			function flash(btn, label, message) {
				btn.textContent = message;
				setTimeout(function () { btn.textContent = label; }, 1500);
			}

			document.querySelectorAll('[data-copy]').forEach(function (btn) {
				btn.setAttribute('data-label', btn.textContent);
				btn.addEventListener('click', function () {
					var text = document.getElementById(btn.getAttribute('data-copy')).textContent.trim();
					var label = btn.getAttribute('data-label') || btn.textContent;
					copyText(text).then(function () {
						flash(btn, label, 'Copied');
					}, function () {
						flash(btn, label, 'Copy failed');
					});
				});
			});
		})();
	</script>
</body></html>`

//go:embed static/header.html
var Header string

//go:embed static/index.html
var MainPage string

//go:embed static/tx.html
var TxPage string

//go:embed static/block.html
var BlockPage string

//go:embed static/mempool.html
var mempoolTemplate string

var MempoolPage string

//go:embed static/style.css
var StyleSheetPage []byte

//go:embed static/logo.svg
var LogoSVG []byte

func InitPages() {
	MainPage = Header + MainPage + Footer
	TxPage = Header + TxPage + Footer
	BlockPage = Header + BlockPage + Footer
	MempoolPage = Header + mempoolTemplate + Footer

}
