// Copyright 2026 Florian Zenker (flo@znkr.io)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package eval runs the SSA IR the analyzer produced and returns the document
// it builds.
//
// [Eval] takes an [expr.Module] and returns the realized document along with
// any warnings and errors. [EvalExports] does the same for a module compiled
// as a library, returning its top-level bindings as well.
//
// Free names are already resolved to constants by the analyzer, so evaluation
// keeps no scope of its own.
//
// # Error handling
//
// A failed instruction does not stop evaluation. It records a [value.Error] and
// writes that same error into its own result, so later instructions reading it
// produce it in turn without recording anything new. One failure is therefore
// reported once, however far its value travels.
//
// A few instructions — [expr.ContentResult], [expr.CodeJoin], [expr.JoinAdd] —
// opt out of that, so a document whose one content block failed still renders
// the rest.
package eval
