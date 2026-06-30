;;; geas.el --- Major mode for the Good Ethereum Assembler  -*- lexical-binding: t; -*-

;; Copyright 2026 The go-ethereum Authors

;; Author: Felix Lange <fjl@twurst.com>
;; Version: 0.1
;; Package-Requires: ((emacs "29.1"))
;; Keywords: languages
;; URL: https://github.com/fjl/geas
;; Assisted-by: Claude Opus 4.8 (1M context)

;; This file is part of the go-ethereum library.
;;
;; The go-ethereum library is free software: you can redistribute it and/or modify
;; it under the terms of the GNU Lesser General Public License as published by
;; the Free Software Foundation, either version 3 of the License, or
;; (at your option) any later version.
;;
;; The go-ethereum library is distributed in the hope that it will be useful,
;; but WITHOUT ANY WARRANTY; without even the implied warranty of
;; MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
;; GNU Lesser General Public License for more details.

;;; Commentary:

;; Major mode for EVM assembly source source files (.eas).
;;
;; The mode is derived from `asm-mode' and adds several features specific
;; to geas:
;;
;;   - imenu for labels and macros
;;   - reformatting via `geas -f'
;;   - context-aware completion of opcodes, macros and labels
;;   - eldoc display of opcode stack effects
;;
;; In order for most features to work, `geas-command' must be set to
;; the path of a working geas binary (or geas must be in PATH).
;;
;; A note about comment-column: the geas formatter usually aligns all
;; end-of-line comments based on the width of the code. However, it can
;; sometimes be nice to set it explicitly. geas.el will honor the
;; `comment-column' variable when it is set locally, which can be done
;; using `set-comment-column' or via file-local/dir-local variables.

;;; Code:

(require 'asm-mode)
(require 'cl-lib)

(defgroup geas nil
  "Major mode for editing geas EVM assembly."
  :group 'languages
  :prefix "geas-")

(defcustom geas-command "geas"
  "Path to the geas executable, used for formatting."
  :type 'string)

(defconst geas-font-lock-additions
  '(;; #-directives.
    ("\\(#\\(?:define\\|include\\|pragma\\|bytes\\)\\)\\_>"
     (1 font-lock-preprocessor-face))
    ;; Label definitions (incl. dotted and named #bytes).
    ("^[ \t]*\\(\\.?[A-Za-z_][A-Za-z0-9_]*\\)[ \t]*:"
     (1 font-lock-function-name-face))
    ;; Label references.
    ("@\\.?[A-Za-z_][A-Za-z0-9_]*" . font-lock-constant-face)
    ;; Macro names.
    ("%[A-Za-z_][A-Za-z0-9_]*" . font-lock-type-face)
    ;; Macro parameters.
    ("\\$[A-Za-z_][A-Za-z0-9_]*" . font-lock-variable-name-face))
  "Geas-specific font-lock keywords layered on top of `asm-mode'.")

(defun geas--comment-column ()
  "Comment column to use for formatting.
This is `comment-column' when it has been set buffer-locally, otherwise 0"
  (if (local-variable-p 'comment-column) comment-column 0))

(defun geas-format-buffer (&optional col)
  "Reformat the current buffer with `geas -f'.
COL is the comment column; it defaults to `comment-column' when that has
been set buffer-locally, otherwise 0 (auto).  With a prefix
argument, prompt for the comment-column."
  (interactive
   (list (if current-prefix-arg
             (read-number "Comment column (0 = auto): " (geas--comment-column))
           (geas--comment-column))))
  (let* ((col (or col (geas--comment-column)))
         (args (append '("-f")
                       (and (> col 0)
                            (list "-col" (number-to-string col)))
                       '("-")))
         (outbuf (generate-new-buffer " *geas-format-output*"))
         (errfile (make-temp-file "geas-format"))
         (coding-system-for-read 'utf-8-unix)
         (coding-system-for-write 'utf-8-unix))
    (unwind-protect
        (let ((status (apply #'call-process-region (point-min) (point-max)
                             geas-command nil (list outbuf errfile) nil args)))
          (if (and (integerp status) (zerop status))
              (replace-buffer-contents outbuf)
            (with-current-buffer (get-buffer-create "*geas-format-errors*")
              (let ((inhibit-read-only t))
                (erase-buffer)
                (insert-file-contents errfile)
                (goto-char (point-min))))
            (display-buffer "*geas-format-errors*")
            (message "geas: formatting failed (%s)" status)))
      (kill-buffer outbuf)
      (delete-file errfile))))

;;;###autoload
(defun geas-format-before-save ()
  "Reformat the buffer if it is in `geas-mode'.
Not enabled by default.  Add it to `before-save-hook' to format
geas files on save, like gofmt:

  (add-hook \\='before-save-hook #\\='geas-format-before-save)"
  (interactive)
  (when (derived-mode-p 'geas-mode)
    (geas-format-buffer)))

(defcustom geas-indent-offset 4
  "Number of columns to indent instructions in `geas-mode'."
  :type 'integer)

(defun geas-calculate-indentation ()
  "Return the indentation column for the current line.
Point must be at the first non-whitespace character of the line.
Directives, a closing `}', labels and top-level comments are
flush left; everything else is indented `geas-indent-offset'."
  (if (or (looking-at-p "[#}]")
          (looking-at-p ";;;")
          (looking-at-p "\\.?\\(?:\\sw\\|\\s_\\)+[ \t]*:"))
      0
    geas-indent-offset))

(defun geas-indent-line ()
  "Indent the current line for `geas-mode'."
  (let* ((savep (point))
         (indent (save-excursion
                   (forward-line 0)
                   (skip-chars-forward " \t")
                   (when (>= (point) savep) (setq savep nil))
                   (max (geas-calculate-indentation) 0))))
    (if savep
        (save-excursion (indent-line-to indent))
      (indent-line-to indent))))

;;; Opcode information from the geas tool.

(defvar geas--stack-cache (make-hash-table :test 'equal)
  "Cache of opcode stack effects, keyed by uppercase opcode name.
Each value is the stack-effect string.")

(defvar geas--opcode-cache (make-hash-table :test 'equal)
  "Cache of opcode-name lists for completion, keyed by target fork name.")

(defun geas-clear-cache ()
  "Clear cached opcode information (stack effects and completion lists).
Use this after upgrading the geas executable."
  (interactive)
  (clrhash geas--stack-cache)
  (clrhash geas--opcode-cache))

(defun geas--target ()
  "Return the target fork from the buffer's `#pragma target', or \"all\".
When no pragma is found, the special target \"all\" is used, covering all
opcodes across all forks."
  (save-excursion
    (goto-char (point-min))
    (if (re-search-forward
         "^[ \t]*#pragma[ \t]+target[ \t]+\"\\([^\"]+\\)\"" nil t)
        (match-string-no-properties 1)
      "all")))

(defun geas--query-stack (opcode callback)
  "Run `geas -i -stack OPCODE' asynchronously and call CALLBACK with the result.
CALLBACK receives the stack-effect string, or nil if OPCODE is unknown
or the command fails."
  (condition-case nil
      (make-process
       :name "geas-stack"
       :buffer (generate-new-buffer " *geas-stack*")
       :command (list geas-command "-i" "-stack" opcode)
       :noquery t
       :connection-type 'pipe
       :sentinel
       (lambda (proc _event)
         (when (memq (process-status proc) '(exit signal))
           (unwind-protect
               (let ((effect nil))
                 ;; Output is "NAME [...] -> [...]"; strip the opcode name.
                 (when (eq 0 (process-exit-status proc))
                   (with-current-buffer (process-buffer proc)
                     (goto-char (point-min))
                     (when (looking-at "[^ \n]+ \\(.*\\)$")
                       (setq effect (match-string-no-properties 1)))))
                 (funcall callback effect))
             (kill-buffer (process-buffer proc))))))
    (error (funcall callback nil))))

(defun geas--opcode-on-line ()
  "Return the opcode on the current line, or nil.
This is the first word of the line, together with any immediate in
brackets (e.g. \"dupn[17]\"), unless the line is a comment, label,
directive or macro invocation."
  (save-excursion
    (beginning-of-line)
    (skip-chars-forward " \t")
    (and (looking-at "\\([A-Za-z][A-Za-z0-9]*\\(?:\\[[^]]*\\]\\)?\\)\\(?:[ \t]\\|$\\)")
         (match-string-no-properties 1))))

(defun geas--get-stack-effect (opcode callback)
  "Call CALLBACK with the stack-effect string for OPCODE, or nil if unknown."
  ;; Note: variable-size `push' pseudo-op is not a real opcode, so we
  ;; translate it here.
  (let* ((name (let ((u (upcase opcode))) (if (string= u "PUSH") "PUSH1" u)))
         (cached (gethash name geas--stack-cache)))
    (if (not cached)
        (geas--query-stack
         name
         (lambda (effect)
           (when effect
             (puthash name effect geas--stack-cache))
           (funcall callback effect)))
      (funcall callback cached))))

(defun geas-show-stack-effect ()
  "Display the stack effect of the opcode on the current line."
  (interactive)
  (let ((op (geas--opcode-on-line)))
    (if (null op)
        (message "No opcode on this line")
      (geas--get-stack-effect
       op
       (lambda (effect)
         (if effect
             (message "%s %s" op effect)
           (message "No stack effect for %s" op)))))))

(defun geas-eldoc-function (callback &rest _)
  "Eldoc function reporting the stack effect of the current line's opcode.
Intended for `eldoc-documentation-functions'."
  (when-let* ((op (geas--opcode-on-line)))
    (geas--get-stack-effect
     op
     (lambda (effect)
       (funcall callback effect :thing op)))
    t))

(defun geas--opcode-names (target)
  "Return the list of opcode names available in TARGET, cached.
Names are lowercased, matching the conventional casing.  Runs
`geas -i -ops TARGET'."
  (or (gethash target geas--opcode-cache)
      (let ((names (ignore-errors
                     (with-temp-buffer
                       (when (eq 0 (call-process geas-command nil t nil
                                                 "-i" "-ops" target))
                         (let (ops)
                           (goto-char (point-min))
                           (while (not (eobp))
                             (push (downcase (buffer-substring-no-properties
                                              (line-beginning-position)
                                              (line-end-position)))
                                   ops)
                             (forward-line 1))
                           (nreverse ops)))))))
        (when names
          (puthash target names geas--opcode-cache))
        names)))

(defun geas--scan-buffer (regexp)
  "Return an alist of (NAME . POSITION) for REGEXP group-1 matches in the buffer.
NAME is the matched text and POSITION its start.  This doubles as both a
completion collection (the names are the keys) and an imenu index."
  (let (result)
    (save-excursion
      (goto-char (point-min))
      (while (re-search-forward regexp nil t)
        (push (cons (match-string-no-properties 1) (match-beginning 1)) result)))
    (nreverse result)))

(defun geas--expression-macros ()
  "Return expression macros defined in the buffer as an (NAME . POSITION) alist."
  (geas--scan-buffer "^[ \t]*#define[ \t]+\\([A-Za-z_][A-Za-z0-9_]*\\)"))

(defun geas--instruction-macros ()
  "Return instruction macros (names with leading %) defined in the buffer.
The result is an (NAME . POSITION) alist."
  (geas--scan-buffer "^[ \t]*#define[ \t]+\\(%[A-Za-z_][A-Za-z0-9_]*\\)"))

(defun geas--labels ()
  "Return labels defined in the buffer as an (NAME . POSITION) alist.
NAME is as it appears after `@': dotted labels keep their leading dot.
Named `#bytes' labels are included."
  (geas--scan-buffer
   "^[ \t]*\\(?:#bytes[ \t]+\\)?\\(\\.?[A-Za-z_][A-Za-z0-9_]*\\)[ \t]*:"))

(defun geas-imenu-create-index ()
  "Build an imenu index of macros and labels for `geas-mode'.
Used as `imenu-create-index-function'."
  (let ((macros (append (geas--expression-macros) (geas--instruction-macros)))
        (labels (geas--labels)))
    (append (and macros (list (cons "Macro" macros)))
            (and labels (list (cons "Label" labels))))))

(defun geas--opcode-position-p (pos)
  "Return non-nil if POS is the first token of its line.
That is the opcode position; otherwise point is in expression context (an
argument of an opcode or macro)."
  (save-excursion
    (goto-char pos)
    (skip-chars-backward " \t")
    (bolp)))

(defun geas-completion-at-point ()
  "Completion-at-point function for `geas-mode'.
Completes opcodes and instruction macros in opcode position (the first
word of a line), and expression macros and labels elsewhere.  Intended
for `completion-at-point-functions'."
  (unless (nth 8 (syntax-ppss))         ; not in a comment or string
    (let* ((end (point))
           (start (save-excursion (skip-chars-backward "A-Za-z0-9_") (point)))
           (before (char-before start)))
      (cl-flet ((capf (beg table) (list beg end table :exclusive 'no)))
        (cond
         ;; @label / @.label -> labels.
         ((eq before ?@)
          (capf start (geas--labels)))
         ((and (eq before ?.) (eq (char-before (1- start)) ?@))
          (capf (1- start) (geas--labels)))
         ;; %macro at opcode position -> instruction macros.
         ((eq before ?%)
          (when (geas--opcode-position-p (1- start))
            (capf (1- start) (geas--instruction-macros))))
         ;; First word of the line -> opcodes.
         ((geas--opcode-position-p start)
          (capf start (geas--opcode-names (geas--target))))
         ;; Otherwise an opcode argument -> expression macros.
         (t
          (capf start (geas--expression-macros))))))))

(defvar geas-mode-map (make-sparse-keymap)
  "Keymap for `geas-mode'.")

(keymap-set geas-mode-map "C-c C-f" #'geas-format-buffer)
(keymap-set geas-mode-map "C-c C-s" #'geas-show-stack-effect)
(keymap-set geas-mode-map ";" #'asm-comment)
;; Override insertion for `:'. asm-mode installs a command for this
;; that tries to be smart but doesn't work well for geas, so we have
;; undo the binding.
(keymap-set geas-mode-map ":" #'self-insert-command)

;;;###autoload
(define-derived-mode geas-mode asm-mode "Geas"
  "Major mode for editing geas EVM assembly, derived from `asm-mode'."
  ;; asm-mode's body installs its own local map via `use-local-map'.
  ;; Re-assert ours in the after-hook, which runs after all of mode
  ;; initialization (including the parent's and any mode hooks), so it is
  ;; the last word on the local map regardless of keymap ordering.
  :after-hook (use-local-map geas-mode-map)
  (font-lock-add-keywords nil geas-font-lock-additions)
  (setq-local imenu-create-index-function #'geas-imenu-create-index)
  (setq-local comment-start "; ")
  (setq-local comment-start-skip ";+[ \t]*")
  (setq-local indent-line-function #'geas-indent-line)
  ;; geas -f uses spaces
  (setq-local indent-tabs-mode nil)
  ;; Re-indent the line when a flush-left character is typed.
  (setq-local electric-indent-chars (append '(?# ?} ?:) electric-indent-chars))
  (add-hook 'eldoc-documentation-functions #'geas-eldoc-function nil t)
  (add-hook 'completion-at-point-functions #'geas-completion-at-point nil t))

;;;###autoload
(add-to-list 'auto-mode-alist '("\\.eas\\'" . geas-mode))

(provide 'geas)

;;; geas.el ends here
