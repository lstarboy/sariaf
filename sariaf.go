// Copyright 2020 Majid Sajadi. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be found
// in the LICENSE file.

package sariaf

import (
	"context"
	"net/http"
	"strings"
)

// each node represent a path in the router trie.
type node struct {
	path     string
	key      string
	children map[string]*node
	handler  http.HandlerFunc
	param    string
}

// add method adds a new path to the trie.
func (n *node) add(path string, handler http.HandlerFunc) {
	current := n

	trimmed := strings.TrimPrefix(path, "/")
	slice := strings.Split(trimmed, "/")

	for _, k := range slice {
		// replace keys with pattern ":*" with "*" for matching params.
		var param string
		if len(k) > 1 && string(k[0]) == ":" {
			param = strings.TrimPrefix(k, ":")
			k = "*"
		}

		next, ok := current.children[k]
		if !ok {
			next = &node{
				path:     path,
				key:      k,
				children: make(map[string]*node),
				param:    param,
			}
			current.children[k] = next
		}
		current = next
	}

	current.handler = handler
}

// find method match the request url path with a node in trie.
func (n *node) find(path string) (*node, Params) {
	params := make(Params)
	current := n

	trimmed := strings.TrimPrefix(path, "/")
	slice := strings.Split(trimmed, "/")

	for _, k := range slice {
		var next *node

		next, ok := current.children[k]
		if !ok {
			next, ok = current.children["*"]
			if !ok {
				// return nil if no node match the given path.
				return nil, params
			}

		}

		current = next

		// if the node has a param add it to params map.
		if current.param != "" {
			params[current.param] = k
		}
	}

	// return the found node and params map.
	return current, params
}

type contextKeyType struct{}

// Params is the type for request params.
type Params map[string]string

// contextKey is the context key for the params.
var contextKey = contextKeyType{}

// newContext returns a new Context that carries a provided params value.
func newContext(ctx context.Context, params Params) context.Context {
	return context.WithValue(ctx, contextKey, params)
}

// fromContext extracts params from a Context.
func fromContext(ctx context.Context) (Params, bool) {
	values, ok := ctx.Value(contextKey).(Params)

	return values, ok
}

// Router is an HTTP request multiplexer. It matches the URL of each
// incoming request against a list of registered path with their associated
// methods and calls the handler for the given URL.
type Router struct {
	trees       map[string]*node
	middlewares []func(http.HandlerFunc) http.HandlerFunc
}

// New returns a new Router.
func New() *Router {
	return &Router{
		trees: make(map[string]*node),
	}
}

// ServeHTTP matches r.URL.Path with a stored route and calls handler for found node.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// check if there is a trie for the request method.
	if _, ok := r.trees[req.Method]; !ok {
		http.NotFound(w, req)
		return
	}

	// find the node with request url path in the trie.
	node, params := r.trees[req.Method].find(req.URL.Path)

	if node != nil && node.handler != nil {
		// attach the params context to request if any param exists.
		if len(params) != 0 {
			ctx := newContext(req.Context(), params)
			req = req.WithContext(ctx)
		}

		// call the middlewares on handler
		var handler = node.handler
		for _, middleware := range r.middlewares {
			handler = middleware(handler)
		}

		// call the node handler
		handler(w, req)
		return
	}

	// call the not found handler if can match the request url path to any node in trie.
	http.NotFound(w, req)
}

// Handle registers a new path with the given path and method.
func (r *Router) Handle(method string, path string, handler http.HandlerFunc) {
	// check if for given method there is not any tie create a new one.
	if _, ok := r.trees[method]; !ok {
		r.trees[method] = &node{
			path:     "/",
			children: make(map[string]*node),
		}
	}

	r.trees[method].add(path, handler)
}

// GetParams returns params stored in the request.
func GetParams(r *http.Request) (Params, bool) {
	return fromContext(r.Context())
}

// Use append middlewares to the middleware stack.
func (r *Router) Use(middlewares ...func(http.HandlerFunc) http.HandlerFunc) {
	if len(middlewares) > 0 {
		r.middlewares = append(r.middlewares, middlewares...)
	}
}
