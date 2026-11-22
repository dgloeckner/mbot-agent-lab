# Team page build output

`npx nx run web:build` writes the team page to `dist/app/`, which the server embeds.
This file keeps the directory present so the server compiles without a frontend build.
