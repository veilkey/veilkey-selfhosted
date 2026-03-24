/// Minimal ANSI escape sequence tokenizer for secret masking.
///
/// Splits byte streams into Text and Escape segments so that secret
/// matching can operate on plaintext while preserving ANSI codes.

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum SegmentKind {
    Text,
    Escape,
}

#[derive(Debug, Clone)]
pub struct Segment {
    pub kind: SegmentKind,
    pub data: Vec<u8>,
}

pub struct Tokenizer;

impl Tokenizer {
    /// Tokenize a byte slice into Text and Escape segments.
    ///
    /// Recognizes CSI sequences (\x1b[...X), OSC sequences (\x1b]...ST),
    /// and two-byte sequences (\x1bX). Everything else is Text.
    pub fn tokenize(input: &[u8]) -> Vec<Segment> {
        let mut segments = Vec::new();
        let mut i = 0;
        let mut text_start = 0;

        while i < input.len() {
            if input[i] == 0x1b {
                // Flush any pending text
                if i > text_start {
                    segments.push(Segment {
                        kind: SegmentKind::Text,
                        data: input[text_start..i].to_vec(),
                    });
                }
                let esc_start = i;
                i += 1;
                if i < input.len() {
                    if input[i] == b'[' {
                        // CSI sequence: \x1b[ params final_byte
                        i += 1;
                        while i < input.len() && (input[i] < 0x40 || input[i] > 0x7e) {
                            i += 1;
                        }
                        if i < input.len() {
                            i += 1; // consume final byte
                        }
                    } else if input[i] == b']' {
                        // OSC sequence: \x1b] ... (ST = \x1b\\ or BEL = \x07)
                        i += 1;
                        while i < input.len() {
                            if input[i] == 0x07 {
                                i += 1;
                                break;
                            }
                            if input[i] == 0x1b && i + 1 < input.len() && input[i + 1] == b'\\' {
                                i += 2;
                                break;
                            }
                            i += 1;
                        }
                    } else {
                        // Two-byte escape: \x1bX
                        i += 1;
                    }
                }
                segments.push(Segment {
                    kind: SegmentKind::Escape,
                    data: input[esc_start..i].to_vec(),
                });
                text_start = i;
            } else {
                i += 1;
            }
        }
        // Flush remaining text
        if text_start < input.len() {
            segments.push(Segment {
                kind: SegmentKind::Text,
                data: input[text_start..].to_vec(),
            });
        }
        segments
    }

    /// Strip all ANSI escape sequences from input, returning only plaintext bytes.
    pub fn strip_ansi(input: &[u8]) -> Vec<u8> {
        let segments = Self::tokenize(input);
        let mut out = Vec::new();
        for seg in &segments {
            if seg.kind == SegmentKind::Text {
                out.extend_from_slice(&seg.data);
            }
        }
        out
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_tokenize_plain_text() {
        let segs = Tokenizer::tokenize(b"hello world");
        assert_eq!(segs.len(), 1);
        assert_eq!(segs[0].kind, SegmentKind::Text);
        assert_eq!(segs[0].data, b"hello world");
    }

    #[test]
    fn test_tokenize_csi() {
        let segs = Tokenizer::tokenize(b"\x1b[31mred\x1b[0m");
        assert_eq!(segs.len(), 3);
        assert_eq!(segs[0].kind, SegmentKind::Escape);
        assert_eq!(segs[1].kind, SegmentKind::Text);
        assert_eq!(segs[1].data, b"red");
        assert_eq!(segs[2].kind, SegmentKind::Escape);
    }

    #[test]
    fn test_strip_ansi() {
        let input = b"\x1b[1m\x1b[36mVK:LOCAL:abc\x1b[0m";
        let plain = Tokenizer::strip_ansi(input);
        assert_eq!(plain, b"VK:LOCAL:abc");
    }

    #[test]
    fn test_tokenize_mixed() {
        let segs = Tokenizer::tokenize(b"pre\x1b[31mmid\x1b[0mpost");
        assert_eq!(segs.len(), 5);
        assert_eq!(segs[0].kind, SegmentKind::Text);
        assert_eq!(segs[0].data, b"pre");
        assert_eq!(segs[2].kind, SegmentKind::Text);
        assert_eq!(segs[2].data, b"mid");
        assert_eq!(segs[4].kind, SegmentKind::Text);
        assert_eq!(segs[4].data, b"post");
    }

    #[test]
    fn test_strip_grep_color() {
        let input = b"KEY=\x1b[01;31m\x1b[Ksk-abc123\x1b[m\x1b[K";
        let plain = Tokenizer::strip_ansi(input);
        assert_eq!(plain, b"KEY=sk-abc123");
    }
}
