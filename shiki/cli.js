import {createHighlighter} from 'shiki'
import {readFileSync, writeFileSync} from 'fs'

const in_file = process.argv[2];
const out_file = process.argv[3];
const language = process.argv[4];

const highlighter = await createHighlighter({
    themes: ['one-dark-pro', 'one-light'],
    langs: [language],
})

const code = readFileSync(in_file, 'utf8');

const highlightedCode = highlighter.codeToHtml(code, {
    lang: language,
    themes: {
        light: 'one-light',
        dark: 'one-dark-pro',
    }
})

writeFileSync(out_file, highlightedCode);
