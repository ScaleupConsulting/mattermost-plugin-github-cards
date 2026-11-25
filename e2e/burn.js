// Burns captions.json into a caption band added under the recorded video, so captions never cover the page.
const fs = require('fs');
const {execFileSync} = require('child_process');
const [dir, out] = process.argv.slice(2);
const caps = JSON.parse(fs.readFileSync(`${dir}/captions.json`, 'utf8'));
const W = 1280, H = 860, BAND = 64;
const cs = (t) => Math.round(t * 100);
const fmt = (c) => `${Math.floor(c / 360000)}:${String(Math.floor(c / 6000) % 60).padStart(2, '0')}:${String(Math.floor(c / 100) % 60).padStart(2, '0')}.${String(c % 100).padStart(2, '0')}`;
let prevEnd = 0;
const events = caps.map((c, i) => {
    const start = Math.max(cs(c.start), prevEnd);
    const end = i + 1 < caps.length ? cs(caps[i + 1].start) : cs(c.end);
    if (start > end) throw new Error(`cue ${i} overlaps`);
    prevEnd = end;
    return `Dialogue: 0,${fmt(start)},${fmt(end)},Default,,0,0,0,,${c.text}`;
});
const ass = `[Script Info]
ScriptType: v4.00+
PlayResX: ${W}
PlayResY: ${H + BAND}
WrapStyle: 2

[V4+ Styles]
Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding
Style: Default,DejaVu Sans,24,&H00FFFFFF,&H00FFFFFF,&H00000000,&H00000000,0,0,0,0,100,100,0,0,1,0,0,2,24,24,20,1

[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
${events.join('\n')}
`;
fs.writeFileSync(`${dir}/captions.ass`, ass);
const webm = fs.readdirSync(dir).find((f) => f.endsWith('.webm'));
execFileSync('ffmpeg', ['-y', '-loglevel', 'error', '-i', `${dir}/${webm}`, '-vf',
    `pad=${W}:${H + BAND}:0:0:0x1f2328,ass=${dir}/captions.ass`, '-c:v', 'libx264', '-pix_fmt', 'yuv420p', '-crf', '22', out], {stdio: 'inherit'});
console.log(JSON.stringify({out, cues: events.length, lastEnd: prevEnd / 100}));
