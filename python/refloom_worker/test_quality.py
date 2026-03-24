"""Tests for text quality detection."""

from refloom_worker.quality import looks_text_corrupt, classify_extraction, _sample_positions


class TestLooksTextCorrupt:
    def test_clean_japanese_text(self):
        assert looks_text_corrupt("これは正常な日本語テキストです。") is False

    def test_clean_english_text(self):
        assert looks_text_corrupt("This is normal English text.") is False

    def test_empty_string(self):
        assert looks_text_corrupt("") is False

    def test_replacement_chars(self):
        """U+FFFD replacement characters indicate mojibake."""
        text = "normal" + "\ufffd" * 5 + "text"
        assert looks_text_corrupt(text) is True

    def test_control_char_run(self):
        """3+ consecutive control characters trigger corrupt."""
        text = "hello" + "\x01\x02\x03" + "world"
        assert looks_text_corrupt(text) is True

    def test_few_suspicious_below_threshold(self):
        """Single suspicious char in long text is OK."""
        text = "a" * 200 + "\ufffd" + "b" * 200
        assert looks_text_corrupt(text) is False

    def test_high_ratio_triggers(self):
        """2%+ suspicious ratio triggers corrupt."""
        # 10 suspicious in 100 printable = 10% > 2%
        text = ("ab\ufffd" * 10) + ("normal" * 20)
        # This has 10 suspicious in ~130 printable
        assert looks_text_corrupt(text) is True

    def test_whitespace_only(self):
        assert looks_text_corrupt("   \n\t  ") is False


class TestClassifyExtraction:
    def test_empty_pages(self):
        assert classify_extraction([]) == "extract_failed"

    def test_all_empty_text(self):
        pages = [{"text": ""}, {"text": "  "}, {"text": "\n"}]
        assert classify_extraction(pages) == "ocr_required"

    def test_clean_pages(self):
        pages = [{"text": "これは正常なテキスト。"} for _ in range(10)]
        assert classify_extraction(pages) == "ok"

    def test_corrupt_pages(self):
        corrupt = "\ufffd" * 50
        pages = [{"text": corrupt} for _ in range(10)]
        assert classify_extraction(pages) == "text_corrupt"

    def test_mixed_mostly_clean(self):
        """1 corrupt page out of 10 is OK."""
        pages = [{"text": "正常なテキスト。"} for _ in range(9)]
        pages.append({"text": "\ufffd" * 50})
        assert classify_extraction(pages) == "ok"

    def test_single_page_ok(self):
        pages = [{"text": "Hello world."}]
        assert classify_extraction(pages) == "ok"


class TestSamplePositions:
    def test_empty(self):
        assert _sample_positions(0) == []

    def test_small(self):
        assert _sample_positions(3) == [0, 1, 2]

    def test_large(self):
        positions = _sample_positions(100)
        assert 0 in positions
        assert 99 in positions
        assert len(positions) <= 9
