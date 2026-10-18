# ForteX teaching tools

## Brand
Preserve ForteX's educational blue identity. Semantic tokens come from globals.css and Tailwind mappings from tailwind.config.ts.

## Typography
Inter/system-ui body, Plus Jakarta Sans/Inter headings. Support Vietnamese. Body line-height >= 1.5; headings >= 1.15. Slide text must be concise and legible on a projector.

## Surfaces
Background, card, foreground, muted, border, primary and ring support light and dark. Scoped --tool-* and --slide-* tokens add presentation surfaces and themes.

## Structure
Slides use 16:9, thumbnails and speaker notes. Quizzes have exactly four options and one correct answer per question, plus an explanation. The application renders structured JSON. Do not generate HTML or executable scripts.

## Shape and spacing
Controls 10px, cards 16px, dialogs 24px. Spacing uses 4px steps and an 8px rhythm through --space-*.

## Accessibility
Use semantic labels, keyboard focus, accessible contrast and responsive forms. Trial quizzes grade locally after submission.

## Workflow
Inspired by nexu-io/open-design (Apache-2.0): source documents + brief -> slides or quiz -> draft -> preview -> refine -> export. Verify correct answers against source documents. The teacher reviews before use. Never claim publication to a course.
