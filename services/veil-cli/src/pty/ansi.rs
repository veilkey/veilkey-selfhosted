/// Streaming ANSI escape sequence tokenizer.
///
/// Separates raw terminal output into text segments and ANSI escape sequences,
/// so secret matching only runs against plaintext — never corrupted by embedded
/// escape codes (e.g. `\x1b[31msecr\x1b[0met` → text "secr" + text "et").

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

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum State {
    Text,
    EscStart,
    Csi,
    Osc,
    OscEsc,
    Dcs,
    DcsEsc,
    Sos,
    SosEsc,
    Pm,
    PmEsc,
    Apc,
    ApcEsc,
}

pub struct Tokenizer {
    state: State,
    esc_buf: Vec<u8>,
    segments: Vec<Segment>,
    text_buf: Vec<u8>,
}

impl Default for Tokenizer {
    fn default() -> Self {
        Self::new()
    }
}

impl Tokenizer {
    pub fn new() -> Self {
        Self {
            state: State::Text,
            esc_buf: Vec::new(),
            segments: Vec::new(),
            text_buf: Vec::new(),
        }
    }

    pub fn push(&mut self, data: &[u8]) {
        for &b in data {
            match self.state {
                State::Text => {
                    if b == 0x1b {
                        self.state = State::EscStart;
                        self.esc_buf.clear();
                        self.esc_buf.push(b);
                    } else {
                        self.text_buf.push(b);
                    }
                }
                State::EscStart => {
                    self.esc_buf.push(b);
                    match b {
                        b'[' => self.state = State::Csi,
                        b']' => self.state = State::Osc,
                        b'P' => self.state = State::Dcs,
                        b'X' => self.state = State::Sos,
                        b'^' => self.state = State::Pm,
                        b'_' => self.state = State::Apc,
                        _ => self.emit_escape(),
                    }
                }
                State::Csi => {
                    self.esc_buf.push(b);
                    if (0x40..=0x7e).contains(&b) {
                        self.emit_escape();
                    }
                }
                State::Osc => {
                    self.esc_buf.push(b);
                    if b == 0x07 {
                        self.emit_escape();
                    } else if b == 0x1b {
                        self.state = State::OscEsc;
                    }
                }
                State::OscEsc => {
                    self.esc_buf.push(b);
                    if b == b'\\' {
                        self.emit_escape();
                    } else {
                        self.state = State::Osc;
                    }
                }
                State::Dcs => {
                    self.esc_buf.push(b);
                    if b == 0x1b {
                        self.state = State::DcsEsc;
                    }
                }
                State::DcsEsc => {
                    self.esc_buf.push(b);
                    if b == b'\\' {
                        self.emit_escape();
                    } else {
                        self.state = State::Dcs;
                    }
                }
                State::Sos => {
                    self.esc_buf.push(b);
                    if b == 0x1b {
                        self.state = State::SosEsc;
                    }
                }
                State::SosEsc => {
                    self.esc_buf.push(b);
                    if b == b'\\' {
                        self.emit_escape();
                    } else {
                        self.state = State::Sos;
                    }
                }
                State::Pm => {
                    self.esc_buf.push(b);
                    if b == 0x1b {
                        self.state = State::PmEsc;
                    }
                }
                State::PmEsc => {
                    self.esc_buf.push(b);
                    if b == b'\\' {
                        self.emit_escape();
                    } else {
                        self.state = State::Pm;
                    }
                }
                State::Apc => {
                    self.esc_buf.push(b);
                    if b == 0x1b {
                        self.state = State::ApcEsc;
                    }
                }
                State::ApcEsc => {
                    self.esc_buf.push(b);
                    if b == b'\\' {
                        self.emit_escape();
                    } else {
                        self.state = State::Apc;
                    }
                }
            }
        }
    }

    pub fn flush(&mut self) {
        self.flush_text();
        if !self.esc_buf.is_empty() {
            self.segments.push(Segment {
                kind: SegmentKind::Escape,
                data: std::mem::take(&mut self.esc_buf),
            });
            self.state = State::Text;
        }
    }

    pub fn drain(&mut self) -> Vec<Segment> {
        std::mem::take(&mut self.segments)
    }

    pub fn tokenize(data: &[u8]) -> Vec<Segment> {
        let mut t = Self::new();
        t.push(data);
        t.flush();
        t.drain()
    }

    pub fn strip_ansi(data: &[u8]) -> Vec<u8> {
        let segments = Self::tokenize(data);
        let mut out = Vec::with_capacity(data.len());
        for seg in &segments {
            if seg.kind == SegmentKind::Text {
                out.extend_from_slice(&seg.data);
            }
        }
        out
    }

    fn flush_text(&mut self) {
        if !self.text_buf.is_empty() {
            self.segments.push(Segment {
                kind: SegmentKind::Text,
                data: std::mem::take(&mut self.text_buf),
            });
        }
    }

    fn emit_escape(&mut self) {
        self.flush_text();
        self.segments.push(Segment {
            kind: SegmentKind::Escape,
            data: std::mem::take(&mut self.esc_buf),
        });
        self.state = State::Text;
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn plain_text_only() {
        let segs = Tokenizer::tokenize(b"hello world");
        assert_eq!(segs.len(), 1);
        assert_eq!(segs[0].kind, SegmentKind::Text);
        assert_eq!(segs[0].data, b"hello world");
    }

    #[test]
    fn single_csi_color() {
        let segs = Tokenizer::tokenize(b"\x1b[31mred\x1b[0m");
        assert_eq!(segs.len(), 3);
        assert_eq!(segs[0].kind, SegmentKind::Escape);
        assert_eq!(segs[0].data, b"\x1b[31m");
        assert_eq!(segs[1].kind, SegmentKind::Text);
        assert_eq!(segs[1].data, b"red");
        assert_eq!(segs[2].kind, SegmentKind::Escape);
        assert_eq!(segs[2].data, b"\x1b[0m");
    }

    #[test]
    fn secret_split_by_color() {
        let input = b"\x1b[31msecr\x1b[0met";
        let text: Vec<u8> = Tokenizer::tokenize(input)
            .iter()
            .filter(|s| s.kind == SegmentKind::Text)
            .flat_map(|s| s.data.iter().copied())
            .collect();
        assert_eq!(text, b"secret");
    }

    #[test]
    fn osc_with_bel() {
        let segs = Tokenizer::tokenize(b"\x1b]0;my title\x07rest");
        assert_eq!(segs.len(), 2);
        assert_eq!(segs[0].kind, SegmentKind::Escape);
        assert_eq!(segs[1].kind, SegmentKind::Text);
        assert_eq!(segs[1].data, b"rest");
    }

    #[test]
    fn osc_with_st() {
        let segs = Tokenizer::tokenize(b"\x1b]0;title\x1b\\rest");
        assert_eq!(segs[0].kind, SegmentKind::Escape);
        assert_eq!(segs[1].kind, SegmentKind::Text);
        assert_eq!(segs[1].data, b"rest");
    }

    #[test]
    fn two_byte_escape() {
        let segs = Tokenizer::tokenize(b"\x1bcafter");
        assert_eq!(segs[0].kind, SegmentKind::Escape);
        assert_eq!(segs[0].data, b"\x1bc");
        assert_eq!(segs[1].kind, SegmentKind::Text);
        assert_eq!(segs[1].data, b"after");
    }

    #[test]
    fn csi_split_across_pushes() {
        let mut tok = Tokenizer::new();
        tok.push(b"pre\x1b[31");
        tok.push(b"mred\x1b[0m");
        tok.flush();
        let segs = tok.drain();
        assert_eq!(segs.len(), 4);
        assert_eq!(segs[0].data, b"pre");
        assert_eq!(segs[1].data, b"\x1b[31m");
        assert_eq!(segs[2].data, b"red");
        assert_eq!(segs[3].data, b"\x1b[0m");
    }

    #[test]
    fn strip_ansi_removes_all_escapes() {
        let input = b"\x1b[1m\x1b[36mVK:LOCAL:abc\x1b[0m and \x1b[31mmore\x1b[0m";
        assert_eq!(Tokenizer::strip_ansi(input), b"VK:LOCAL:abc and more");
    }

    #[test]
    fn complex_prompt() {
        let input = b"\x1b[01;32muser@host\x1b[00m:\x1b[01;34m~/dir\x1b[00m$ ";
        let text: Vec<u8> = Tokenizer::tokenize(input)
            .iter()
            .filter(|s| s.kind == SegmentKind::Text)
            .flat_map(|s| s.data.iter().copied())
            .collect();
        assert_eq!(text, b"user@host:~/dir$ ");
    }

    #[test]
    fn empty_input() {
        assert_eq!(Tokenizer::tokenize(b"").len(), 0);
    }

    #[test]
    fn dcs_sequence() {
        let segs = Tokenizer::tokenize(b"\x1bPsome data\x1b\\after");
        assert_eq!(segs[0].kind, SegmentKind::Escape);
        assert_eq!(segs[1].kind, SegmentKind::Text);
        assert_eq!(segs[1].data, b"after");
    }

    #[test]
    fn multiple_csi_interleaved() {
        let input = b"a\x1b[1mb\x1b[2mc\x1b[3md";
        let text: String = Tokenizer::tokenize(input)
            .iter()
            .filter(|s| s.kind == SegmentKind::Text)
            .map(|s| String::from_utf8_lossy(&s.data).to_string())
            .collect();
        assert_eq!(text, "abcd");
    }
}
