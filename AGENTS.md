# figma-make-app

React + Vite + Tailwind CSS project running inside Figma Make.

## Development Server

A Vite development server is **already running** on `$PORT` (default 8443). You don't need to start it manually.

- Preview URL: The user can access the running app through the preview panel
- Hot reload: Changes to files are reflected immediately

## Project Structure

This is the canonical project structure. Start with task-relevant files below. Only follow imports or inspect other files when required, when a documented path is missing, or when the repository contradicts this guide.

- `src/main.tsx` - React entrypoint; imports `src/index.css` and mounts `src/App.tsx` into the `#root` element
- `src/App.tsx` - Primary application component and the usual starting point for UI work
- `src/index.css` - Global CSS entrypoint and Tailwind CSS v4 import
- `index.html` - Vite HTML shell containing the `#root` element and loading `src/main.tsx`
- `package.json` - Project dependencies and the Vite build, development, preview, and formatting scripts
- `vite.config.ts` - Vite configuration with React, Tailwind CSS v4, and Figma Make plugins plus the `@` alias for `src`

## Dependencies

- Runtime: React 19 and React DOM 19
- Styling: Tailwind CSS v4 through the `@tailwindcss/vite` plugin configured in `vite.config.ts`
- Build tooling: Vite 8, TypeScript 5.7, and `@vitejs/plugin-react`
- Formatting: oxfmt

## Styling

This project uses **Tailwind CSS v4** through the `@tailwindcss/vite` plugin. `src/index.css` imports Tailwind with `@import 'tailwindcss';`. Use Tailwind utility classes directly in JSX and put global CSS or Tailwind v4 theme customization in `src/index.css`. This scaffold does not need a Tailwind config file or PostCSS config.

`src/main.tsx` imports `src/index.css`, so global font wiring belongs in `src/index.css`. Keep CSS `@import` statements first, then add any `@font-face` rules and font-family defaults there.

## GitHub workflow

GitHub Issues are the canonical tracker for this repository.

- Use the GitHub CLI (`gh`) for GitHub operations: inspect repositories and issues, create or edit tickets, add comments, inspect pull requests, and update or close remote work.
- Ongoing implementation tickets belong in GitHub Issues in `HenriqueMichelini/careeros`. Do not create or modify ticket Markdown under `.scratch/`.
- Before starting work, find the relevant issue with `gh issue list` and read it with `gh issue view`. Preserve the issue's acceptance criteria, blockers, evidence requirements, and domain terms.
- For implementation, work in a local branch, run the required checks, commit the source changes, push the branch, and use `gh pr` to create or update the pull request. Do not mutate project files on the default branch through the GitHub web UI or direct Contents/API writes.
- Keep issue progress in the issue itself: use `gh issue comment`, `gh issue edit`, and `gh issue close` as appropriate. Do not maintain a second local ticket state.
- If `gh` is unavailable, do not silently fall back to `.scratch/` or direct remote file writes. Report the environment limitation or use an explicitly approved GitHub integration that performs the equivalent Issue/PR operation.
