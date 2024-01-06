const fs = require('fs');

const prismDarkCss = fs.readFileSync('node_modules/prismjs/themes/prism-dark.css', 'utf8');
const prismLightCss = fs.readFileSync('node_modules/prismjs/themes/prism.css', 'utf8');

const newPrismDarkCss = `@media (prefers-color-scheme: dark) {\n${prismDarkCss}\n}`;
const newPrismCss = `@media (prefers-color-scheme: light) {\n${prismLightCss}\n}`;

fs.writeFileSync('prism-dark-light.css', `${newPrismDarkCss}\n${newPrismCss}`);
