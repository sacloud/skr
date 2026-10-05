from __future__ import annotations

import http.server
import pathlib
import tempfile
import threading
import unittest

import check_tutorial_links


class Handler(http.server.BaseHTTPRequestHandler):
    def do_GET(self) -> None:
        if self.path == "/ok":
            self.send_response(200)
            self.end_headers()
            self.wfile.write(b"ok")
            return
        if self.path == "/redirect":
            self.send_response(302)
            self.send_header("Location", "/ok")
            self.end_headers()
            return
        self.send_response(404)
        self.end_headers()

    def log_message(self, format: str, *args: object) -> None:
        return


class CheckTutorialLinksTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        cls.thread = threading.Thread(target=cls.server.serve_forever, daemon=True)
        cls.thread.start()
        cls.base_url = f"http://127.0.0.1:{cls.server.server_port}"

    @classmethod
    def tearDownClass(cls) -> None:
        cls.server.shutdown()
        cls.server.server_close()
        cls.thread.join()

    def test_extracts_markdown_links_and_skips_code(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = pathlib.Path(directory) / "tutorial.md"
            path.write_text(
                f"[first]({self.base_url}/ok)\n"
                f"<{self.base_url}/redirect>\n"
                f"![image]({self.base_url}/image.png)\n"
                f"`[{self.base_url}/inline-code]`\n"
                f"```shell\n[{self.base_url}/code-block]\n```\n",
                encoding="utf-8",
            )

            links, local = check_tutorial_links.extract_links(path)

        self.assertEqual([1], [item.line for item in links[f"{self.base_url}/ok"]])
        self.assertIn(f"{self.base_url}/redirect", links)
        self.assertIn(f"{self.base_url}/image.png", links)
        self.assertEqual([], local)
        self.assertNotIn(f"{self.base_url}/inline-code", links)
        self.assertNotIn(f"{self.base_url}/code-block", links)

    def test_check_link_accepts_success_and_redirect(self) -> None:
        success = check_tutorial_links.check_link(f"{self.base_url}/ok", timeout=1, retries=0)
        redirect = check_tutorial_links.check_link(
            f"{self.base_url}/redirect", timeout=1, retries=0
        )
        self.assertTrue(success.ok)
        self.assertTrue(redirect.ok)
        self.assertEqual(f"{self.base_url}/ok", redirect.final_url)

    def test_check_link_rejects_missing_page(self) -> None:
        result = check_tutorial_links.check_link(
            f"{self.base_url}/missing", timeout=1, retries=0
        )
        self.assertFalse(result.ok)
        self.assertEqual(404, result.status)

    def test_local_links_must_resolve_to_files(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            target = root / "reference.md"
            target.write_text("# Reference\n", encoding="utf-8")
            tutorial = root / "tutorial.md"
            tutorial.write_text(
                "[valid](reference.md) [missing](missing.md) [section](#overview)\n",
                encoding="utf-8",
            )

            _, links = check_tutorial_links.extract_links(tutorial)
            self.assertEqual(["reference.md", "missing.md"], [link.target for link in links])
            self.assertTrue(check_tutorial_links.local_link_exists(links[0]))
            self.assertFalse(check_tutorial_links.local_link_exists(links[1]))

    def test_default_paths_includes_resource_tutorials(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            nested = root / "iaas-api" / "switch.md"
            nested.parent.mkdir()
            nested.write_text("# Switch\n", encoding="utf-8")
            top_level = root / "eventbus-api.md"
            top_level.write_text("# EventBus\n", encoding="utf-8")

            self.assertEqual(
                sorted([nested, top_level]),
                check_tutorial_links.default_paths(root),
            )


if __name__ == "__main__":
    unittest.main()
