--
-- PostgreSQL database dump
--

\restrict El7wLBoi5G9jzH6bRSIEXpa4L1g1HX4PQRwJr3jjtTe2OjNYGxcwbrlUgkvmgrT

-- Dumped from database version 18.4
-- Dumped by pg_dump version 18.4

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET transaction_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: public; Type: SCHEMA; Schema: -; Owner: -
--

-- *not* creating schema, since initdb creates it


--
-- Name: SCHEMA public; Type: COMMENT; Schema: -; Owner: -
--

COMMENT ON SCHEMA public IS '';


--
-- Name: chk_project_job_share_total(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.chk_project_job_share_total() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE v_total numeric;
BEGIN
  SELECT COALESCE(SUM(share_percent), 0) INTO v_total
  FROM public.project_job_share WHERE project_id = NEW.project_id;
  IF v_total > 100 THEN
    RAISE EXCEPTION 'project_job_share total % exceeds 100 for project %', v_total, NEW.project_id;
  END IF;
  RETURN NULL;
END;
$$;


--
-- Name: enforce_read_before_write(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.enforce_read_before_write() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  IF NEW.can_read = false THEN
    NEW.can_write  := false;
    NEW.can_update := false;
    NEW.can_delete := false;
  END IF;
  RETURN NEW;
END;
$$;


--
-- Name: trg_project_cost_budget_item_only(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.trg_project_cost_budget_item_only() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    v_head  boolean;
    v_major boolean;
BEGIN
    SELECT is_head, is_major INTO v_head, v_major
    FROM public.cost_subgroup WHERE id = NEW.cost_subgroup_id;

    IF v_head OR v_major THEN
        RAISE EXCEPTION 'งบประมาณกำหนดได้เฉพาะรหัสต้นทุนระดับรายการเท่านั้น (ไม่ใช่รหัสระดับ Head หรือ Major)';
    END IF;
    RETURN NEW;
END;
$$;


SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: approval_config; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.approval_config (
    id bigint NOT NULL,
    doc_type character varying(30) NOT NULL,
    step_no integer NOT NULL,
    step_name character varying(200) NOT NULL,
    approver_role_id bigint,
    min_amount numeric(18,4),
    max_amount numeric(18,4),
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint
);


--
-- Name: approval_config_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.approval_config ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.approval_config_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: approval_delegation; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.approval_delegation (
    id bigint NOT NULL,
    doc_type character varying(30),
    user_id bigint NOT NULL,
    reason text,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint
);


--
-- Name: approval_delegation_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.approval_delegation ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.approval_delegation_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: approval_doc_types; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.approval_doc_types (
    id bigint NOT NULL,
    doc_type character varying(30) NOT NULL,
    doc_label character varying(200) NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint,
    table_name character varying(50),
    id_column character varying(50) DEFAULT 'id'::character varying,
    status_column character varying(50) DEFAULT 'status'::character varying,
    approved_value character varying(30) DEFAULT 'APPROVED'::character varying,
    rejected_value character varying(30) DEFAULT 'REJECTED'::character varying,
    status_log_table character varying(50),
    status_log_fk_column character varying(50)
);


--
-- Name: approval_doc_types_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.approval_doc_types ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.approval_doc_types_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: approval_log; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.approval_log (
    id bigint NOT NULL,
    approval_id bigint NOT NULL,
    doc_type character varying(30) NOT NULL,
    doc_id bigint NOT NULL,
    doc_no character varying(30) NOT NULL,
    step_no integer NOT NULL,
    action character varying(20) NOT NULL,
    action_by bigint NOT NULL,
    action_at timestamp without time zone DEFAULT now() NOT NULL,
    comments text,
    old_status character varying(20),
    new_status character varying(20),
    ip_address inet,
    user_agent text,
    CONSTRAINT approval_log_action_check CHECK (((action)::text = ANY (ARRAY['SUBMIT'::text, 'APPROVE'::text, 'REJECT'::text, 'RETURN'::text, 'CANCEL'::text, 'ESCALATE'::text, 'COMMENT'::text])))
);


--
-- Name: approval_log_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.approval_log ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.approval_log_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: approval_request; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.approval_request (
    id bigint NOT NULL,
    doc_type character varying(30) NOT NULL,
    doc_id bigint NOT NULL,
    doc_no character varying(30) NOT NULL,
    step_no integer NOT NULL,
    requested_by bigint NOT NULL,
    assigned_to bigint,
    status character varying(20) DEFAULT 'PENDING'::character varying NOT NULL,
    due_date date,
    amount numeric(18,4),
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint,
    CONSTRAINT approval_request_status_check CHECK (((status)::text = ANY (ARRAY['PENDING'::text, 'APPROVED'::text, 'REJECTED'::text, 'CANCELLED'::text, 'ESCALATED'::text])))
);


--
-- Name: approval_request_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.approval_request ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.approval_request_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: borrow; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.borrow (
    id bigint NOT NULL,
    borrow_no character varying(30) NOT NULL,
    borrow_type character varying(20) DEFAULT 'BORROW'::character varying NOT NULL,
    warehouse_code character varying(20) NOT NULL,
    borrower_id bigint NOT NULL,
    location_code character varying(20),
    borrow_date date DEFAULT CURRENT_DATE NOT NULL,
    expected_return date,
    actual_return date,
    status character varying(20) DEFAULT 'OPEN'::character varying NOT NULL,
    remarks text,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint NOT NULL,
    updated_by bigint,
    purpose text,
    approved_by bigint,
    approved_at timestamp without time zone,
    project_code character varying(50),
    CONSTRAINT borrow_status_check CHECK (((status)::text = ANY (ARRAY['DRAFT'::text, 'PENDING_APPROVAL'::text, 'APPROVED'::text, 'REJECTED'::text, 'BORROWED'::text, 'RETURNED'::text, 'PARTIALLY_RETURNED'::text, 'CANCELLED'::text]))),
    CONSTRAINT borrow_type_check CHECK (((borrow_type)::text = ANY (ARRAY['BORROW'::text, 'RETURN'::text])))
);


--
-- Name: COLUMN borrow.purpose; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.borrow.purpose IS 'วัตถุประสงค์การยืม';


--
-- Name: COLUMN borrow.approved_by; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.borrow.approved_by IS 'ผู้อนุมัติ';


--
-- Name: borrow_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.borrow ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.borrow_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: borrow_line; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.borrow_line (
    id bigint NOT NULL,
    borrow_id bigint NOT NULL,
    mat_code character varying(20) NOT NULL,
    mat_type character varying(20) DEFAULT 'RETURNABLE'::character varying NOT NULL,
    qty_borrowed numeric(18,4) NOT NULL,
    qty_returned numeric(18,4) DEFAULT 0 NOT NULL,
    condition_out character varying(50),
    condition_in character varying(50),
    remarks text,
    stock_item_id bigint,
    location_code character varying(20),
    qty_requested numeric(18,4),
    qty_approved numeric(18,4),
    line_no integer,
    unit character varying(50),
    CONSTRAINT borrow_line_condition_in_check CHECK (((condition_in)::text = ANY (ARRAY['GOOD'::text, 'FAIR'::text, 'DAMAGED'::text]))),
    CONSTRAINT borrow_line_condition_out_check CHECK (((condition_out)::text = ANY (ARRAY['GOOD'::text, 'FAIR'::text, 'DAMAGED'::text]))),
    CONSTRAINT borrow_line_mat_type_check CHECK (((mat_type)::text = ANY (ARRAY['RETURNABLE'::text, 'CONSUMABLE'::text]))),
    CONSTRAINT borrow_line_qty_borrowed_check CHECK ((qty_borrowed > (0)::numeric))
);


--
-- Name: COLUMN borrow_line.stock_item_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.borrow_line.stock_item_id IS 'FK → stock_item (ใหม่) แทน mat_code';


--
-- Name: COLUMN borrow_line.qty_requested; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.borrow_line.qty_requested IS 'จำนวนที่ขอยืม';


--
-- Name: COLUMN borrow_line.qty_approved; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.borrow_line.qty_approved IS 'จำนวนที่อนุมัติ (อาจน้อยกว่าที่ขอ)';


--
-- Name: borrow_line_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.borrow_line ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.borrow_line_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: borrow_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.borrow_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: borrow_status_log; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.borrow_status_log (
    id bigint NOT NULL,
    borrow_id bigint NOT NULL,
    from_status character varying(30),
    to_status character varying(30) NOT NULL,
    changed_by bigint NOT NULL,
    changed_at timestamp without time zone DEFAULT now() NOT NULL,
    remarks text
);


--
-- Name: TABLE borrow_status_log; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.borrow_status_log IS 'ประวัติการเปลี่ยนสถานะการยืม';


--
-- Name: borrow_status_log_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.borrow_status_log ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.borrow_status_log_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: brand; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.brand (
    id integer NOT NULL,
    brand_code character varying(10) NOT NULL,
    spec_id integer NOT NULL,
    brand_name character varying(100) NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint
);


--
-- Name: brand_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.brand_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: brand_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.brand_id_seq OWNED BY public.brand.id;


--
-- Name: cost_group; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cost_group (
    id integer NOT NULL,
    job_id integer NOT NULL,
    group_code character varying(5) NOT NULL,
    group_name character varying(100) NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint
);


--
-- Name: cost_group_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.cost_group_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: cost_group_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.cost_group_id_seq OWNED BY public.cost_group.id;


--
-- Name: cost_job; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cost_job (
    id integer NOT NULL,
    subject_id integer NOT NULL,
    job_code character varying(5) NOT NULL,
    job_name character varying(100) NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint
);


--
-- Name: cost_job_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.cost_job_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: cost_job_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.cost_job_id_seq OWNED BY public.cost_job.id;


--
-- Name: cost_subgroup; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cost_subgroup (
    id integer NOT NULL,
    group_id integer NOT NULL,
    subgroup_code character varying(5) NOT NULL,
    subgroup_name character varying(100),
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint,
    is_head boolean DEFAULT false NOT NULL,
    is_major boolean DEFAULT false NOT NULL,
    CONSTRAINT chk_cost_subgroup_major_not_head CHECK ((NOT (is_major AND is_head)))
);


--
-- Name: cost_subgroup_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.cost_subgroup_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: cost_subgroup_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.cost_subgroup_id_seq OWNED BY public.cost_subgroup.id;


--
-- Name: cost_subject; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cost_subject (
    id integer NOT NULL,
    subject_code character varying(5) NOT NULL,
    subject_name character varying(100) NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint
);


--
-- Name: cost_subject_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.cost_subject_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: cost_subject_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.cost_subject_id_seq OWNED BY public.cost_subject.id;


--
-- Name: customer; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.customer (
    cus_id integer NOT NULL,
    customer_code character varying(20) NOT NULL,
    customer_name character varying(255) NOT NULL,
    address text,
    contact text,
    credit_term character varying(50),
    remarks text,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    tel character varying(50),
    updated_by bigint,
    contact_phone character varying(50),
    tax_id character varying(20)
);


--
-- Name: customer_cus_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.customer_cus_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: customer_cus_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.customer_cus_id_seq OWNED BY public.customer.cus_id;


--
-- Name: departments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.departments (
    id bigint NOT NULL,
    dept_code character varying(50) NOT NULL,
    dept_name character varying(200) NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    short_code character varying(10)
);


--
-- Name: departments_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.departments ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.departments_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: dept_menu_permissions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.dept_menu_permissions (
    id bigint NOT NULL,
    menu_id bigint NOT NULL,
    can_read boolean DEFAULT false NOT NULL,
    can_write boolean DEFAULT false NOT NULL,
    can_update boolean DEFAULT false NOT NULL,
    can_delete boolean DEFAULT false NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint,
    dept_code character varying(50) NOT NULL
);


--
-- Name: dept_menu_permissions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.dept_menu_permissions ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.dept_menu_permissions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: doc_number_counter; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.doc_number_counter (
    prefix text NOT NULL,
    year_month text NOT NULL,
    last_seq integer DEFAULT 0 NOT NULL
);


--
-- Name: erp_audit_log; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.erp_audit_log (
    id bigint NOT NULL,
    table_name character varying(100) NOT NULL,
    record_id bigint NOT NULL,
    action character varying(10) NOT NULL,
    changed_by bigint,
    changed_at timestamp without time zone DEFAULT now() NOT NULL,
    old_data jsonb,
    new_data jsonb,
    session_info jsonb,
    CONSTRAINT erp_audit_log_action_check CHECK (((action)::text = ANY (ARRAY['INSERT'::text, 'UPDATE'::text, 'DELETE'::text])))
);


--
-- Name: erp_audit_log_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.erp_audit_log ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.erp_audit_log_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: grn; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.grn (
    id bigint NOT NULL,
    grn_no character varying(30) NOT NULL,
    grn_date date DEFAULT CURRENT_DATE NOT NULL,
    po_id bigint NOT NULL,
    warehouse_code character varying(20) NOT NULL,
    delivery_note character varying(100),
    status character varying(20) DEFAULT 'DRAFT'::character varying NOT NULL,
    quality_status character varying(20) DEFAULT 'PENDING'::character varying NOT NULL,
    received_by bigint NOT NULL,
    confirmed_by bigint,
    confirmed_at timestamp without time zone,
    remarks text,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint,
    score_quality smallint,
    score_quantity smallint,
    score_ontime smallint,
    score_notes text,
    supplier_id integer,
    invoice_no character varying(100),
    delivery_date date,
    CONSTRAINT grn_quality_check CHECK (((quality_status)::text = ANY (ARRAY['PENDING'::text, 'PASSED'::text, 'FAILED'::text, 'PARTIAL'::text]))),
    CONSTRAINT grn_score_ontime_check CHECK (((score_ontime IS NULL) OR ((score_ontime >= 1) AND (score_ontime <= 5)))),
    CONSTRAINT grn_score_quality_check CHECK (((score_quality IS NULL) OR ((score_quality >= 1) AND (score_quality <= 5)))),
    CONSTRAINT grn_score_quantity_check CHECK (((score_quantity IS NULL) OR ((score_quantity >= 1) AND (score_quantity <= 5)))),
    CONSTRAINT grn_status_check CHECK (((status)::text = ANY (ARRAY['DRAFT'::text, 'CONFIRMED'::text, 'POSTED'::text])))
);


--
-- Name: COLUMN grn.score_quality; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.grn.score_quality IS 'คะแนนคุณภาพสินค้า 1-5 ให้หลังรับของเข้าระบบแล้ว (nullable จนกว่าจะให้คะแนน)';


--
-- Name: COLUMN grn.score_quantity; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.grn.score_quantity IS 'คะแนนความครบถ้วนของจำนวน 1-5';


--
-- Name: COLUMN grn.score_ontime; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.grn.score_ontime IS 'คะแนนความตรงเวลาในการส่งของ 1-5';


--
-- Name: grn_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.grn ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.grn_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: grn_line; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.grn_line (
    id bigint NOT NULL,
    grn_id bigint NOT NULL,
    line_no integer NOT NULL,
    po_line_id bigint NOT NULL,
    mat_code character varying(20) NOT NULL,
    zone_id integer,
    qty_received numeric(18,4) NOT NULL,
    qty_accepted numeric(18,4) DEFAULT 0 NOT NULL,
    qty_rejected numeric(18,4) DEFAULT 0 NOT NULL,
    quality_remarks text,
    CONSTRAINT grn_line_qty_received_check CHECK ((qty_received >= (0)::numeric))
);


--
-- Name: grn_line_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.grn_line ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.grn_line_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: grn_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.grn_seq
    START WITH 10
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: ic_po_receive_document; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ic_po_receive_document (
    id bigint NOT NULL,
    po_id bigint NOT NULL,
    tax_invoice_no character varying(50),
    tax_invoice_date date,
    temp_delivery_no character varying(50),
    temp_delivery_date date,
    credit_days integer,
    due_date date,
    exchange_rate numeric(18,6),
    remarks text,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint,
    receive_no character varying(30),
    score_quality numeric,
    score_quantity numeric,
    score_ontime numeric,
    score_notes text,
    rated_at timestamp with time zone,
    rated_by bigint,
    CONSTRAINT chk_ic_po_receive_scores CHECK ((((score_quality IS NULL) OR ((score_quality >= (1)::numeric) AND (score_quality <= (5)::numeric))) AND ((score_quantity IS NULL) OR ((score_quantity >= (1)::numeric) AND (score_quantity <= (5)::numeric))) AND ((score_ontime IS NULL) OR ((score_ontime >= (1)::numeric) AND (score_ontime <= (5)::numeric)))))
);


--
-- Name: ic_po_receive_document_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.ic_po_receive_document_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: ic_po_receive_document_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.ic_po_receive_document_id_seq OWNED BY public.ic_po_receive_document.id;


--
-- Name: ic_po_return_document; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ic_po_return_document (
    id bigint NOT NULL,
    po_id bigint NOT NULL,
    return_no character varying(30) NOT NULL,
    return_date date DEFAULT (timezone('Asia/Bangkok'::text, now()))::date NOT NULL,
    remarks text,
    created_by bigint,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL
);


--
-- Name: ic_po_return_document_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.ic_po_return_document ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.ic_po_return_document_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: ic_po_return_line; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ic_po_return_line (
    id bigint NOT NULL,
    return_id bigint NOT NULL,
    line_no integer NOT NULL,
    po_line_id bigint NOT NULL,
    mat_code character varying(30) NOT NULL,
    cost_subgroup_id bigint,
    return_qty numeric(18,4) NOT NULL,
    remarks text,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    CONSTRAINT ic_po_return_line_return_qty_check CHECK ((return_qty > (0)::numeric))
);


--
-- Name: ic_po_return_line_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.ic_po_return_line ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.ic_po_return_line_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: ic_project_cost_item; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ic_project_cost_item (
    id bigint NOT NULL,
    project_code character varying(20) NOT NULL,
    mat_code character varying(20) NOT NULL,
    cost_subgroup_id bigint NOT NULL,
    item_name character varying(255),
    unit character varying(50),
    qty_on_hand numeric(18,4) DEFAULT 0 NOT NULL,
    last_unit_cost numeric(18,4) DEFAULT 0 NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL
);


--
-- Name: ic_project_cost_item_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.ic_project_cost_item_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: ic_project_cost_item_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.ic_project_cost_item_id_seq OWNED BY public.ic_project_cost_item.id;


--
-- Name: ic_project_cost_item_transaction; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ic_project_cost_item_transaction (
    id bigint NOT NULL,
    project_cost_item_id bigint NOT NULL,
    po_id bigint,
    po_line_id bigint,
    qty numeric(18,4) NOT NULL,
    qty_before numeric(18,4) NOT NULL,
    qty_after numeric(18,4) NOT NULL,
    unit_cost numeric(18,4) NOT NULL,
    txn_date date DEFAULT CURRENT_DATE NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    remarks text,
    receive_document_id bigint,
    movement_id bigint,
    movement_line_id bigint,
    ref_type character varying(20) DEFAULT 'PO'::character varying NOT NULL,
    pr_id bigint,
    pr_line_id bigint,
    return_document_id bigint,
    CONSTRAINT chk_ic_project_cost_item_txn_ref_consistency CHECK (((((ref_type)::text = 'PO'::text) AND (po_id IS NOT NULL) AND (po_line_id IS NOT NULL) AND (movement_id IS NULL) AND (movement_line_id IS NULL) AND (pr_id IS NULL) AND (pr_line_id IS NULL)) OR (((ref_type)::text = ANY ((ARRAY['MOVEMENT_ISSUE'::character varying, 'MOVEMENT_TRANSFER'::character varying])::text[])) AND (movement_id IS NOT NULL) AND (movement_line_id IS NOT NULL) AND (po_id IS NULL) AND (po_line_id IS NULL) AND (pr_id IS NULL) AND (pr_line_id IS NULL)) OR (((ref_type)::text = 'PR'::text) AND (pr_id IS NOT NULL) AND (pr_line_id IS NOT NULL) AND (po_id IS NULL) AND (po_line_id IS NULL) AND (movement_id IS NULL) AND (movement_line_id IS NULL))))
);


--
-- Name: ic_project_cost_item_transaction_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.ic_project_cost_item_transaction_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: ic_project_cost_item_transaction_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.ic_project_cost_item_transaction_id_seq OWNED BY public.ic_project_cost_item_transaction.id;


--
-- Name: ic_project_movement; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ic_project_movement (
    id bigint NOT NULL,
    doc_no character varying(30) NOT NULL,
    doc_type character varying(10) NOT NULL,
    doc_date date DEFAULT CURRENT_DATE NOT NULL,
    project_code character varying(20) NOT NULL,
    job_code character varying(20) NOT NULL,
    requested_by bigint NOT NULL,
    status character varying(20) DEFAULT 'DRAFT'::character varying NOT NULL,
    remarks text,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint,
    CONSTRAINT chk_ic_project_movement_doc_type CHECK (((doc_type)::text = ANY ((ARRAY['ISSUE'::character varying, 'TRANSFER'::character varying])::text[])))
);


--
-- Name: ic_project_movement_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.ic_project_movement ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.ic_project_movement_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: ic_project_movement_line; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ic_project_movement_line (
    id bigint NOT NULL,
    movement_id bigint NOT NULL,
    line_no integer NOT NULL,
    mat_code character varying(20) NOT NULL,
    cost_subgroup_id bigint,
    qty numeric(18,4) NOT NULL,
    to_project_code character varying(20) NOT NULL,
    to_cost_subgroup_id bigint,
    remarks text,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    CONSTRAINT chk_ic_project_movement_line_qty_positive CHECK ((qty > (0)::numeric))
);


--
-- Name: ic_project_movement_line_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.ic_project_movement_line ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.ic_project_movement_line_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: ic_project_receipt_pool; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ic_project_receipt_pool (
    id bigint NOT NULL,
    project_code character varying(20) NOT NULL,
    mat_code character varying(20) NOT NULL,
    cost_subgroup_id bigint NOT NULL,
    qty_received numeric(18,4) DEFAULT 0 NOT NULL,
    qty_issued numeric(18,4) DEFAULT 0 NOT NULL,
    item_name character varying(255),
    unit character varying(20),
    last_unit_cost numeric(18,4),
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    CONSTRAINT chk_ic_project_receipt_pool_issued CHECK ((qty_issued <= qty_received))
);


--
-- Name: ic_project_receipt_pool_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.ic_project_receipt_pool ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.ic_project_receipt_pool_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: ic_project_receipt_pool_transaction; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ic_project_receipt_pool_transaction (
    id bigint NOT NULL,
    receipt_pool_id bigint NOT NULL,
    txn_type character varying(20) NOT NULL,
    qty numeric(18,4) NOT NULL,
    ref_type character varying(20) NOT NULL,
    po_id bigint,
    po_line_id bigint,
    receive_document_id bigint,
    movement_id bigint,
    movement_line_id bigint,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    CONSTRAINT chk_ic_project_receipt_pool_txn_type CHECK (((txn_type)::text = ANY (ARRAY[('RETURN'::character varying)::text, ('RECEIVE'::character varying)::text, ('ISSUE_OUT'::character varying)::text])))
);


--
-- Name: ic_project_receipt_pool_transaction_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.ic_project_receipt_pool_transaction ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.ic_project_receipt_pool_transaction_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: inventory; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.inventory (
    id bigint NOT NULL,
    mat_code character varying(20) NOT NULL,
    warehouse_code character varying(20) NOT NULL,
    zone_id integer,
    qty_on_hand numeric(18,4) DEFAULT 0 NOT NULL,
    qty_reserved numeric(18,4) DEFAULT 0 NOT NULL,
    qty_on_order numeric(18,4) DEFAULT 0 NOT NULL,
    reorder_point numeric(18,4),
    reorder_qty numeric(18,4),
    min_stock numeric(18,4),
    max_stock numeric(18,4),
    last_counted_at timestamp without time zone,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint
);


--
-- Name: inventory_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.inventory ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.inventory_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: inventory_transaction; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.inventory_transaction (
    id bigint NOT NULL,
    txn_no character varying(30) NOT NULL,
    txn_type character varying(30) NOT NULL,
    mat_code character varying(20) NOT NULL,
    from_warehouse character varying(20),
    to_warehouse character varying(20),
    from_zone_id integer,
    to_zone_id integer,
    qty numeric(18,4) NOT NULL,
    ref_doc_type character varying(30),
    ref_doc_no character varying(30),
    location_code character varying(20),
    reason text,
    txn_date date DEFAULT CURRENT_DATE NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint NOT NULL,
    updated_by bigint,
    CONSTRAINT inventory_transaction_qty_check CHECK ((qty > (0)::numeric)),
    CONSTRAINT inventory_transaction_txn_type_check CHECK (((txn_type)::text = ANY (ARRAY['ISSUE'::text, 'RETURN'::text, 'TRANSFER_OUT'::text, 'TRANSFER_IN'::text, 'ADJUST_PLUS'::text, 'ADJUST_MINUS'::text, 'GRN_IN'::text, 'BORROW_OUT'::text, 'BORROW_RETURN'::text])))
);


--
-- Name: inventory_transaction_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.inventory_transaction ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.inventory_transaction_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: location; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.location (
    id integer NOT NULL,
    location_code character varying(20) NOT NULL,
    location_name character varying(200) NOT NULL,
    location_type character varying(50) NOT NULL,
    parent_id integer,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint,
    warehouse_code character varying(20),
    CONSTRAINT location_type_check CHECK (((location_type)::text = ANY (ARRAY['department'::text, 'project'::text, 'site'::text])))
);


--
-- Name: location_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.location_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: location_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.location_id_seq OWNED BY public.location.id;


--
-- Name: mat_group; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mat_group (
    id integer NOT NULL,
    group_code character varying(10) NOT NULL,
    group_name character varying(100) NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint
);


--
-- Name: mat_group_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.mat_group_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: mat_group_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.mat_group_id_seq OWNED BY public.mat_group.id;


--
-- Name: mat_name; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mat_name (
    id integer NOT NULL,
    mat_name_code character varying(10) NOT NULL,
    subgroup_id integer NOT NULL,
    mat_name character varying(200) NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint
);


--
-- Name: mat_name_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.mat_name_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: mat_name_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.mat_name_id_seq OWNED BY public.mat_name.id;


--
-- Name: material_code; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.material_code (
    id integer NOT NULL,
    mat_code character varying(20) NOT NULL,
    group_id integer NOT NULL,
    subgroup_id integer NOT NULL,
    mat_name_id integer NOT NULL,
    spec_id integer NOT NULL,
    brand_id integer NOT NULL,
    unit_id integer NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint,
    cost_subgroup_id integer
);


--
-- Name: material_code_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.material_code_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: material_code_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.material_code_id_seq OWNED BY public.material_code.id;


--
-- Name: memo; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.memo (
    id bigint NOT NULL,
    memo_no character varying(30) NOT NULL,
    title character varying(255),
    project_code character varying(20),
    requested_by bigint NOT NULL,
    department character varying(100),
    note text,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint,
    status character varying(30) DEFAULT 'DRAFT'::character varying NOT NULL,
    approver_id bigint,
    delivery_location text,
    site_delivery_date date,
    responsible_factory character varying(255),
    job_code character varying(20),
    CONSTRAINT memo_status_check CHECK (((status)::text = ANY (ARRAY['DRAFT'::text, 'PENDING_APPROVAL'::text, 'APPROVED'::text, 'REJECTED'::text, 'CANCELLED'::text])))
);


--
-- Name: COLUMN memo.status; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.memo.status IS 'DRAFT → PENDING_APPROVAL → APPROVED/REJECTED';


--
-- Name: COLUMN memo.approver_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.memo.approver_id IS 'ผู้อนุมัติที่ถูกกำหนดไว้ตอนสร้าง Memo — เฉพาะคนนี้เท่านั้นที่ approve ได้';


--
-- Name: memo_attachment; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.memo_attachment (
    id bigint NOT NULL,
    memo_id bigint NOT NULL,
    file_path text NOT NULL,
    file_name character varying(255) NOT NULL,
    file_size bigint NOT NULL,
    file_type character varying(20),
    uploaded_by bigint,
    uploaded_at timestamp without time zone DEFAULT now() NOT NULL
);


--
-- Name: TABLE memo_attachment; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.memo_attachment IS 'ไฟล์แนบของ Memo — รูปภาพ, Excel, CSV, PDF';


--
-- Name: memo_attachment_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.memo_attachment ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.memo_attachment_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: memo_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.memo ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.memo_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: memo_line; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.memo_line (
    id bigint NOT NULL,
    memo_id bigint NOT NULL,
    line_no integer NOT NULL,
    description text NOT NULL,
    unit character varying(50) NOT NULL,
    quantity numeric(18,4) NOT NULL,
    remark text,
    CONSTRAINT memo_line_qty_check CHECK ((quantity > (0)::numeric))
);


--
-- Name: memo_line_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.memo_line ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.memo_line_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: memo_number_counter; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.memo_number_counter (
    year_month character varying(6) NOT NULL,
    last_seq integer DEFAULT 0 NOT NULL
);


--
-- Name: memo_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.memo_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: memo_status_log; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.memo_status_log (
    id bigint NOT NULL,
    memo_id bigint NOT NULL,
    from_status character varying(30),
    to_status character varying(30) NOT NULL,
    changed_by bigint NOT NULL,
    changed_at timestamp without time zone DEFAULT now() NOT NULL,
    remarks text
);


--
-- Name: TABLE memo_status_log; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.memo_status_log IS 'ประวัติการเปลี่ยนสถานะของ Memo';


--
-- Name: memo_status_log_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.memo_status_log ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.memo_status_log_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: menus; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.menus (
    id bigint NOT NULL,
    parent_id bigint,
    menu_code character varying(100) NOT NULL,
    menu_name character varying(255) NOT NULL,
    menu_path character varying(255),
    icon_name character varying(100),
    sort_order integer DEFAULT 0,
    is_active boolean DEFAULT true,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint
);


--
-- Name: menus_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.menus ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.menus_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: modules; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.modules (
    id bigint NOT NULL,
    module_code character varying(100) NOT NULL,
    module_name character varying(255) NOT NULL,
    sort_order integer DEFAULT 0,
    is_active boolean DEFAULT true,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint
);


--
-- Name: modules_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.modules ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.modules_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: payment_installment; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.payment_installment (
    id bigint NOT NULL,
    doc_type character varying(10) NOT NULL,
    doc_id bigint NOT NULL,
    installment_no integer NOT NULL,
    percent numeric(7,4),
    amount numeric(18,2) DEFAULT 0 NOT NULL,
    description text,
    is_paid boolean DEFAULT false NOT NULL,
    paid_at timestamp without time zone,
    paid_by bigint,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint,
    payment_log_id bigint,
    CONSTRAINT payment_installment_amount_check CHECK ((amount >= (0)::numeric)),
    CONSTRAINT payment_installment_doc_type_check CHECK (((doc_type)::text = ANY ((ARRAY['PO'::character varying, 'WO'::character varying, 'PC'::character varying])::text[]))),
    CONSTRAINT payment_installment_percent_check CHECK (((percent IS NULL) OR ((percent >= (0)::numeric) AND (percent <= (100)::numeric))))
);


--
-- Name: payment_installment_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.payment_installment ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.payment_installment_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: payment_log; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.payment_log (
    id bigint NOT NULL,
    doc_type character varying(30) NOT NULL,
    doc_id bigint NOT NULL,
    doc_no character varying(30) NOT NULL,
    amount_paid numeric(18,2) NOT NULL,
    paid_date date DEFAULT CURRENT_DATE NOT NULL,
    paid_by bigint NOT NULL,
    remark text,
    reverses_id bigint,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint NOT NULL,
    CONSTRAINT payment_log_amount_paid_check CHECK ((amount_paid <> (0)::numeric)),
    CONSTRAINT payment_log_amount_sign_chk CHECK ((((reverses_id IS NULL) AND (amount_paid > (0)::numeric)) OR ((reverses_id IS NOT NULL) AND (amount_paid < (0)::numeric)))),
    CONSTRAINT payment_log_doc_type_check CHECK (((doc_type)::text = ANY ((ARRAY['PO'::character varying, 'WO'::character varying, 'PC'::character varying])::text[])))
);


--
-- Name: payment_log_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.payment_log ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.payment_log_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: pc_number_counter; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.pc_number_counter (
    year_month character varying(6) NOT NULL,
    last_seq integer DEFAULT 0 NOT NULL
);


--
-- Name: permission_audit_logs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.permission_audit_logs (
    id bigint NOT NULL,
    user_id bigint,
    action character varying(100),
    target_type character varying(100),
    target_id bigint,
    old_value jsonb,
    new_value jsonb,
    created_at timestamp without time zone DEFAULT now() NOT NULL
);


--
-- Name: permission_audit_logs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.permission_audit_logs ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.permission_audit_logs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: permissions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.permissions (
    id bigint NOT NULL,
    permission_key character varying(200) NOT NULL,
    permission_name character varying(255),
    module_id bigint NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint
);


--
-- Name: permissions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.permissions ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.permissions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: petty_cash_attachment; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.petty_cash_attachment (
    id bigint NOT NULL,
    pc_id bigint NOT NULL,
    file_name character varying(255) NOT NULL,
    file_path text NOT NULL,
    file_size bigint,
    file_type character varying(100),
    uploaded_at timestamp without time zone DEFAULT now() NOT NULL,
    uploaded_by bigint
);


--
-- Name: petty_cash_attachment_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.petty_cash_attachment ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.petty_cash_attachment_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: petty_cash_requisition; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.petty_cash_requisition (
    id bigint NOT NULL,
    pc_no character varying(30) NOT NULL,
    pc_date date DEFAULT CURRENT_DATE NOT NULL,
    requested_by bigint NOT NULL,
    purpose text,
    currency character varying(10) DEFAULT 'THB'::character varying NOT NULL,
    total_amount numeric(18,4) DEFAULT 0 NOT NULL,
    use_discount boolean DEFAULT false NOT NULL,
    discount_type character varying(10) DEFAULT 'pct'::character varying NOT NULL,
    discount_amount numeric(18,4) DEFAULT 0 NOT NULL,
    use_vat boolean DEFAULT false NOT NULL,
    vat_amount numeric(18,4) DEFAULT 0 NOT NULL,
    use_wht boolean DEFAULT false NOT NULL,
    wht_amount numeric(18,4) DEFAULT 0 NOT NULL,
    net_amount numeric(18,4) DEFAULT 0 NOT NULL,
    status character varying(20) DEFAULT 'DRAFT'::character varying NOT NULL,
    approver_id bigint,
    remarks text,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint NOT NULL,
    updated_by bigint,
    doc_type character varying(30) DEFAULT 'PETTY_CASH'::character varying NOT NULL,
    branch_code character varying(20),
    payee_emp_code character varying(30),
    payee_name character varying(200),
    project_code character varying(20),
    project_name character varying(200),
    order_type character varying(30),
    dept_code character varying(20),
    dept_name character varying(200),
    ref_doc_no character varying(50),
    approver_remarks text,
    CONSTRAINT chk_pc_discount_type CHECK (((discount_type)::text = ANY ((ARRAY['pct'::character varying, 'amt'::character varying])::text[]))),
    CONSTRAINT chk_pc_status CHECK (((status)::text = ANY ((ARRAY['DRAFT'::character varying, 'PENDING_APPROVAL'::character varying, 'APPROVED'::character varying, 'REJECTED'::character varying, 'CANCELLED'::character varying])::text[]))),
    CONSTRAINT petty_cash_requisition_order_type_check CHECK (((order_type IS NULL) OR ((order_type)::text = ANY ((ARRAY['stock'::character varying, 'cost'::character varying, 'asset_machine'::character varying, 'asset_office_equipment'::character varying, 'asset_tools'::character varying])::text[]))))
);


--
-- Name: petty_cash_requisition_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.petty_cash_requisition ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.petty_cash_requisition_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: petty_cash_requisition_line; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.petty_cash_requisition_line (
    id bigint NOT NULL,
    pc_id bigint NOT NULL,
    line_no integer NOT NULL,
    mat_code character varying(20),
    cost_subgroup_id bigint,
    description text,
    qty numeric(18,4) NOT NULL,
    unit_price numeric(18,4) NOT NULL,
    amount numeric(18,4),
    discount numeric(18,4) DEFAULT 0 NOT NULL,
    disc_type character varying(10) DEFAULT 'pct'::character varying NOT NULL,
    line_discount numeric(18,4) DEFAULT 0 NOT NULL,
    line_vat numeric(18,4) DEFAULT 0 NOT NULL,
    line_wht numeric(18,4) DEFAULT 0 NOT NULL,
    line_net numeric(18,4) DEFAULT 0 NOT NULL,
    wht_rate numeric(5,2),
    remarks text,
    project_code character varying(20),
    supplier_id integer,
    supplier_name character varying(200),
    supplier_address text,
    supplier_tax_id character varying(50),
    has_tax_invoice boolean DEFAULT false NOT NULL,
    tax_invoice_no character varying(50),
    tax_invoice_date date,
    has_wht boolean DEFAULT false NOT NULL,
    wht_base_amount numeric(18,4),
    tax_form_code character varying(20),
    wht_income_type_id integer,
    wht_pay_date date,
    invoice_no character varying(50),
    invoice_date date,
    CONSTRAINT chk_pcl_disc_type CHECK (((disc_type)::text = ANY ((ARRAY['pct'::character varying, 'amt'::character varying])::text[]))),
    CONSTRAINT chk_pcl_name_or_code CHECK (((mat_code IS NOT NULL) OR (length(btrim(COALESCE(description, ''::text))) > 0))),
    CONSTRAINT chk_pcl_tax_form CHECK (((tax_form_code IS NULL) OR ((tax_form_code)::text = ANY ((ARRAY['PND3'::character varying, 'PND53'::character varying])::text[])))),
    CONSTRAINT chk_pcl_wht_fields CHECK (((NOT has_wht) OR ((tax_form_code IS NOT NULL) AND (wht_income_type_id IS NOT NULL) AND (wht_rate IS NOT NULL))))
);


--
-- Name: petty_cash_requisition_line_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.petty_cash_requisition_line ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.petty_cash_requisition_line_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: petty_cash_status_log; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.petty_cash_status_log (
    id bigint NOT NULL,
    pc_id bigint NOT NULL,
    from_status character varying(30),
    to_status character varying(30) NOT NULL,
    changed_by bigint NOT NULL,
    changed_at timestamp without time zone DEFAULT now() NOT NULL,
    remarks text
);


--
-- Name: petty_cash_status_log_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.petty_cash_status_log ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.petty_cash_status_log_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: po_attachment; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.po_attachment (
    id integer NOT NULL,
    po_id bigint NOT NULL,
    file_name character varying NOT NULL,
    file_path character varying NOT NULL,
    file_size bigint,
    file_type character varying,
    uploaded_at timestamp without time zone DEFAULT now(),
    uploaded_by bigint,
    source_pr_attachment_id bigint
);


--
-- Name: po_attachment_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.po_attachment ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.po_attachment_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: po_edit_log; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.po_edit_log (
    id bigint NOT NULL,
    po_id bigint NOT NULL,
    edited_by bigint NOT NULL,
    reason text NOT NULL,
    edited_at timestamp without time zone DEFAULT now() NOT NULL
);


--
-- Name: po_edit_log_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.po_edit_log ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.po_edit_log_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: po_number_counter; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.po_number_counter (
    year_month character varying(6) NOT NULL,
    last_seq integer DEFAULT 0 NOT NULL
);


--
-- Name: po_status_log; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.po_status_log (
    id bigint NOT NULL,
    po_id bigint NOT NULL,
    from_status character varying(30),
    to_status character varying(30) NOT NULL,
    changed_by bigint NOT NULL,
    changed_at timestamp without time zone DEFAULT now() NOT NULL,
    remarks text
);


--
-- Name: po_status_log_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.po_status_log ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.po_status_log_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: por_number_counter; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.por_number_counter (
    year_month character varying(6) NOT NULL,
    last_seq integer DEFAULT 0 NOT NULL
);


--
-- Name: pr_attachment; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.pr_attachment (
    id integer NOT NULL,
    pr_id bigint NOT NULL,
    file_name character varying(255) NOT NULL,
    file_path character varying(500) NOT NULL,
    file_size bigint,
    file_type character varying(100),
    uploaded_at timestamp without time zone DEFAULT now() NOT NULL,
    uploaded_by bigint
);


--
-- Name: pr_attachment_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.pr_attachment_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: pr_attachment_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.pr_attachment_id_seq OWNED BY public.pr_attachment.id;


--
-- Name: pr_number_counter; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.pr_number_counter (
    year_month character varying(6) NOT NULL,
    last_seq integer DEFAULT 0 NOT NULL
);


--
-- Name: pr_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.pr_seq
    START WITH 10
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: pr_status_log; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.pr_status_log (
    id bigint NOT NULL,
    pr_id bigint NOT NULL,
    from_status character varying(30),
    to_status character varying(30) NOT NULL,
    changed_by bigint NOT NULL,
    changed_at timestamp without time zone DEFAULT now() NOT NULL,
    remarks text
);


--
-- Name: pr_status_log_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.pr_status_log ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.pr_status_log_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: project; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.project (
    id integer NOT NULL,
    project_code character varying(20) NOT NULL,
    project_name character varying(200) NOT NULL,
    location_code character varying(500),
    start_date date,
    end_date date,
    status character varying(20) DEFAULT 'ACTIVE'::character varying NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint,
    owner_id bigint,
    budget_amount numeric(18,4) DEFAULT 0 NOT NULL,
    consultant_name character varying(255),
    job_codes text[],
    owner_name character varying(200),
    credit character varying(200),
    dept_code character varying(20),
    responsible_person_name character varying(200),
    consultant_phone character varying(50),
    customer_id bigint,
    warehouse_code character varying(50),
    linked_warehouse_code character varying(20),
    project_kind character varying(10) DEFAULT 'NEW'::character varying NOT NULL,
    contract_amount numeric(18,4) DEFAULT 0 NOT NULL,
    cost_amount numeric(18,4) DEFAULT 0 NOT NULL,
    vo_amount numeric(18,4) DEFAULT 0 NOT NULL,
    CONSTRAINT project_kind_check CHECK (((project_kind)::text = ANY ((ARRAY['NEW'::character varying, 'VO'::character varying])::text[]))),
    CONSTRAINT project_status_check CHECK (((status)::text = ANY ((ARRAY['ACTIVE'::character varying, 'INACTIVE'::character varying, 'CLOSED'::character varying])::text[])))
);


--
-- Name: COLUMN project.owner_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.project.owner_id IS 'ผู้รับผิดชอบหลักของโครงการ — ปกติเป็นทีมวิศวกร';


--
-- Name: COLUMN project.budget_amount; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.project.budget_amount IS 'มูลค่าโครงการ (งบประมาณ)';


--
-- Name: COLUMN project.consultant_name; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.project.consultant_name IS 'ที่ปรึกษาโครงการ (กรอกอิสระ)';


--
-- Name: COLUMN project.dept_code; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.project.dept_code IS 'แผนก — FK to departments.dept_code';


--
-- Name: COLUMN project.responsible_person_name; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.project.responsible_person_name IS 'ผู้รับผิดชอบหลัก — free text, required at the application layer; replaces the owner_id dropdown (owner_id column kept, unused)';


--
-- Name: COLUMN project.consultant_phone; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.project.consultant_phone IS 'เบอร์ติดต่อของที่ปรึกษา — freeform, no validation';


--
-- Name: project_code_counter; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.project_code_counter (
    code_year integer NOT NULL,
    cus_id integer NOT NULL,
    last_no integer DEFAULT 0 NOT NULL
);


--
-- Name: project_code_year_counter; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.project_code_year_counter (
    code_year integer NOT NULL,
    last_no integer DEFAULT 0 NOT NULL
);


--
-- Name: project_cost_budget; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.project_cost_budget (
    id bigint NOT NULL,
    project_id integer NOT NULL,
    cost_subgroup_id integer NOT NULL,
    budget_amount numeric(18,2) DEFAULT 0 NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint,
    CONSTRAINT chk_project_cost_budget_amount_nonneg CHECK ((budget_amount >= (0)::numeric))
);


--
-- Name: project_cost_budget_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.project_cost_budget ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.project_cost_budget_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: project_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.project_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: project_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.project_id_seq OWNED BY public.project.id;


--
-- Name: project_job_share; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.project_job_share (
    project_id bigint NOT NULL,
    job_code character varying(10) NOT NULL,
    share_percent numeric(5,2) NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    CONSTRAINT project_job_share_share_percent_check CHECK (((share_percent > (0)::numeric) AND (share_percent <= (100)::numeric)))
);


--
-- Name: project_responsible_person; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.project_responsible_person (
    project_id bigint NOT NULL,
    user_id bigint NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    created_by bigint
);


--
-- Name: project_stock; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.project_stock (
    id bigint NOT NULL,
    project_code character varying(20) NOT NULL,
    mat_code character varying(30) NOT NULL,
    unit character varying(50),
    qty_on_hand numeric(18,4) DEFAULT 0 NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL
);


--
-- Name: project_stock_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.project_stock ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.project_stock_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: purchase_order; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.purchase_order (
    id bigint NOT NULL,
    po_no character varying(30) NOT NULL,
    po_date date DEFAULT CURRENT_DATE NOT NULL,
    pr_id bigint,
    rfq_id bigint,
    currency character varying(10) DEFAULT 'THB'::character varying NOT NULL,
    total_amount numeric(18,4) DEFAULT 0 NOT NULL,
    vat_amount numeric(18,4) DEFAULT 0 NOT NULL,
    net_amount numeric(18,4) DEFAULT 0 NOT NULL,
    expected_date date,
    status character varying(30) DEFAULT 'DRAFT'::character varying NOT NULL,
    payment_terms character varying(100),
    delivery_address text,
    remarks text,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint NOT NULL,
    updated_by bigint,
    use_discount boolean DEFAULT false NOT NULL,
    discount_type character varying(10) DEFAULT 'pct'::character varying NOT NULL,
    use_vat boolean DEFAULT false NOT NULL,
    use_wht boolean DEFAULT false NOT NULL,
    discount_amount numeric(18,4) DEFAULT 0 NOT NULL,
    wht_amount numeric(18,4) DEFAULT 0 NOT NULL,
    location_code character varying(20),
    location_text character varying(255),
    warehouse_code character varying(20),
    project_code character varying(20),
    requested_by bigint,
    approver_id bigint,
    ref character varying,
    status_receive character varying(20) DEFAULT 'NOT_SENT'::character varying NOT NULL,
    order_type character varying(30) DEFAULT 'stock'::character varying NOT NULL,
    job_code character varying(10) NOT NULL,
    supplier_id integer,
    receiver_name character varying(100),
    receiver_phone character varying(20),
    deleted_at timestamp with time zone,
    deleted_by bigint,
    deposit_paid boolean DEFAULT false NOT NULL,
    deposit_paid_at timestamp without time zone,
    deposit_paid_by bigint,
    CONSTRAINT po_discount_type_check CHECK (((discount_type)::text = ANY ((ARRAY['pct'::character varying, 'amt'::character varying])::text[]))),
    CONSTRAINT po_status_check CHECK (((status)::text = ANY (ARRAY['DRAFT'::text, 'PENDING_APPROVAL'::text, 'APPROVED'::text, 'REJECTED'::text, 'PENDING_REAPPROVAL'::text, 'SENT'::text, 'PARTIALLY_RECEIVED'::text, 'RECEIVED'::text, 'CANCELLED'::text]))),
    CONSTRAINT purchase_order_job_code_check CHECK (((job_code)::text = ANY ((ARRAY['MP'::character varying, 'ME'::character varying, 'MS'::character varying, 'MF'::character varying, 'MG'::character varying, 'MH'::character varying, 'FS'::character varying, 'FP'::character varying, 'FB'::character varying, 'DE'::character varying, 'RE'::character varying, 'G'::character varying])::text[]))),
    CONSTRAINT purchase_order_order_type_check CHECK (((order_type)::text = ANY (ARRAY['stock'::text, 'cost'::text, 'office_equipment'::text, 'asset_equipment'::text, 'asset_tool'::text, 'asset_office_equipment'::text, 'asset_machine'::text, 'asset_tools'::text]))),
    CONSTRAINT purchase_order_status_receive_check CHECK (((status_receive)::text = ANY ((ARRAY['NOT_SENT'::character varying, 'SENT'::character varying, 'PARTIALLY_RECEIVED'::character varying, 'RECEIVED'::character varying])::text[])))
);


--
-- Name: COLUMN purchase_order.use_discount; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.purchase_order.use_discount IS 'เปิดใช้ส่วนลดหรือไม่';


--
-- Name: COLUMN purchase_order.discount_type; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.purchase_order.discount_type IS 'ประเภทส่วนลด: pct = % | amt = ฿';


--
-- Name: COLUMN purchase_order.use_vat; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.purchase_order.use_vat IS 'เปิดใช้ VAT 7% หรือไม่';


--
-- Name: COLUMN purchase_order.use_wht; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.purchase_order.use_wht IS 'เปิดใช้ WHT หรือไม่ (อัตราอยู่ที่แต่ละ line)';


--
-- Name: COLUMN purchase_order.discount_amount; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.purchase_order.discount_amount IS 'ส่วนลดรวมทุก line';


--
-- Name: COLUMN purchase_order.wht_amount; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.purchase_order.wht_amount IS 'WHT รวมทุก line';


--
-- Name: COLUMN purchase_order.location_text; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.purchase_order.location_text IS 'สถานที่ส่งของ — free text, no longer linked to location master';


--
-- Name: COLUMN purchase_order.warehouse_code; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.purchase_order.warehouse_code IS 'คลังสินค้า (optional) — FK to warehouse.warehouse_code, reference only';


--
-- Name: purchase_order_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.purchase_order ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.purchase_order_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: purchase_order_line; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.purchase_order_line (
    id bigint NOT NULL,
    po_id bigint NOT NULL,
    line_no integer NOT NULL,
    mat_code character varying(20) NOT NULL,
    pr_line_id bigint,
    qty_ordered numeric(18,4) NOT NULL,
    qty_received numeric(18,4) DEFAULT 0 NOT NULL,
    unit_price numeric(18,4) NOT NULL,
    amount numeric(18,4) GENERATED ALWAYS AS ((qty_ordered * unit_price)) STORED,
    remarks text,
    status character varying(20) DEFAULT 'OPEN'::character varying NOT NULL,
    description text,
    discount numeric(18,4) DEFAULT 0 NOT NULL,
    wht_rate numeric(5,2),
    line_discount numeric(18,4) DEFAULT 0 NOT NULL,
    line_vat numeric(18,4) DEFAULT 0 NOT NULL,
    line_wht numeric(18,4) DEFAULT 0 NOT NULL,
    line_net numeric(18,4) DEFAULT 0 NOT NULL,
    disc_type character varying(3) DEFAULT 'pct'::character varying NOT NULL,
    cost_subgroup_id bigint,
    CONSTRAINT po_line_status_check CHECK (((status)::text = ANY (ARRAY['OPEN'::text, 'PARTIAL'::text, 'RECEIVED'::text, 'CANCELLED'::text]))),
    CONSTRAINT po_line_wht_rate_check CHECK (((wht_rate IS NULL) OR (wht_rate = ANY (ARRAY[(1)::numeric, (3)::numeric, (5)::numeric])))),
    CONSTRAINT purchase_order_line_qty_ordered_check CHECK ((qty_ordered > (0)::numeric)),
    CONSTRAINT purchase_order_line_unit_price_check CHECK ((unit_price >= (0)::numeric))
);


--
-- Name: COLUMN purchase_order_line.discount; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.purchase_order_line.discount IS 'ค่าส่วนลดที่กรอก (% หรือ ฿ ตาม po.discount_type)';


--
-- Name: COLUMN purchase_order_line.wht_rate; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.purchase_order_line.wht_rate IS 'อัตรา WHT ต่อ row: 1, 3, หรือ 5 (NULL = ไม่หัก)';


--
-- Name: COLUMN purchase_order_line.line_discount; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.purchase_order_line.line_discount IS 'ส่วนลดเป็น ฿ ที่คำนวณแล้ว';


--
-- Name: COLUMN purchase_order_line.line_vat; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.purchase_order_line.line_vat IS 'VAT เป็น ฿ ที่คำนวณแล้ว (7% คงที่)';


--
-- Name: COLUMN purchase_order_line.line_wht; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.purchase_order_line.line_wht IS 'WHT เป็น ฿ ที่คำนวณแล้ว';


--
-- Name: COLUMN purchase_order_line.line_net; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.purchase_order_line.line_net IS 'ยอดสุทธิต่อ row = amount - line_discount + line_vat - line_wht';


--
-- Name: purchase_order_line_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.purchase_order_line ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.purchase_order_line_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: purchase_request; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.purchase_request (
    id bigint NOT NULL,
    pr_no character varying(30) NOT NULL,
    pr_date date DEFAULT CURRENT_DATE NOT NULL,
    requested_by bigint NOT NULL,
    warehouse_code character varying(20),
    required_date date,
    status character varying(30) DEFAULT 'DRAFT'::character varying NOT NULL,
    priority character varying(20) DEFAULT 'NORMAL'::character varying,
    remarks text,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint,
    project_code character varying(20),
    memo_id bigint,
    location_text character varying(255),
    order_type character varying(30) DEFAULT 'stock'::character varying NOT NULL,
    pr_type character varying(10) DEFAULT 'PO_WO'::character varying NOT NULL,
    job_code character varying(10) NOT NULL,
    dept_code character varying(50),
    deleted_at timestamp with time zone,
    deleted_by bigint,
    CONSTRAINT pr_priority_check CHECK (((priority)::text = ANY (ARRAY['LOW'::text, 'NORMAL'::text, 'HIGH'::text, 'URGENT'::text]))),
    CONSTRAINT pr_status_check CHECK (((status)::text = ANY (ARRAY['DRAFT'::text, 'COMPLETED'::text, 'STOCK_CHECK'::text, 'PARTIALLY_FILLED'::text, 'FULFILLED'::text, 'CANCELLED'::text]))),
    CONSTRAINT purchase_request_job_code_check CHECK (((job_code)::text = ANY ((ARRAY['MP'::character varying, 'ME'::character varying, 'MS'::character varying, 'MF'::character varying, 'MG'::character varying, 'MH'::character varying, 'FS'::character varying, 'FP'::character varying, 'FB'::character varying, 'DE'::character varying, 'RE'::character varying, 'G'::character varying])::text[]))),
    CONSTRAINT purchase_request_order_type_check CHECK (((order_type)::text = ANY (ARRAY['stock'::text, 'cost'::text, 'office_equipment'::text, 'asset_equipment'::text, 'asset_tool'::text, 'asset_office_equipment'::text, 'asset_machine'::text, 'asset_tools'::text]))),
    CONSTRAINT purchase_request_pr_type_check CHECK (((pr_type)::text = ANY ((ARRAY['PO_WO'::character varying, 'PO_ONLY'::character varying, 'WO_ONLY'::character varying])::text[])))
);


--
-- Name: COLUMN purchase_request.warehouse_code; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.purchase_request.warehouse_code IS 'คลังสินค้า (optional) — FK to warehouse.warehouse_code, used to deduct stock on PR submit';


--
-- Name: COLUMN purchase_request.memo_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.purchase_request.memo_id IS 'อ้างอิง Memo ต้นทาง (nullable) — ดูว่า PR นี้มาจากใบบันทึกไหน';


--
-- Name: COLUMN purchase_request.location_text; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.purchase_request.location_text IS 'สถานที่ส่งของ — free text, no longer linked to location master';


--
-- Name: purchase_request_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.purchase_request ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.purchase_request_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: purchase_request_line; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.purchase_request_line (
    id bigint NOT NULL,
    pr_id bigint NOT NULL,
    line_no integer NOT NULL,
    mat_code character varying(20) NOT NULL,
    qty_requested numeric(18,4) NOT NULL,
    qty_reserved numeric(18,4) DEFAULT 0 NOT NULL,
    qty_to_order numeric(18,4) DEFAULT 0 NOT NULL,
    remarks text,
    status character varying(30) DEFAULT 'OPEN'::character varying NOT NULL,
    qty_ordered numeric(18,4) DEFAULT 0 NOT NULL,
    cost_subgroup_id bigint,
    deduct_stock boolean DEFAULT true NOT NULL,
    memo_line_id bigint,
    CONSTRAINT pr_line_status_check CHECK (((status)::text = ANY (ARRAY['OPEN'::text, 'RESERVED'::text, 'ORDERED'::text, 'FULFILLED'::text, 'CANCELLED'::text]))),
    CONSTRAINT purchase_request_line_qty_requested_check CHECK ((qty_requested > (0)::numeric))
);


--
-- Name: purchase_request_line_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.purchase_request_line ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.purchase_request_line_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: requisition; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.requisition (
    id bigint NOT NULL,
    req_no character varying(30) NOT NULL,
    project_code character varying(20) NOT NULL,
    warehouse_code character varying(20) NOT NULL,
    requester_id bigint NOT NULL,
    req_date date DEFAULT CURRENT_DATE NOT NULL,
    purpose text,
    status character varying(20) DEFAULT 'DRAFT'::character varying NOT NULL,
    checked_by bigint,
    checked_at timestamp without time zone,
    remarks text,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint NOT NULL,
    updated_by bigint,
    is_stock_house boolean DEFAULT false NOT NULL,
    container_no character varying(100),
    CONSTRAINT chk_requisition_container_no CHECK (((is_stock_house = false) OR (container_no IS NOT NULL))),
    CONSTRAINT requisition_status_check CHECK (((status)::text = ANY ((ARRAY['DRAFT'::character varying, 'SUBMITTED'::character varying, 'ISSUED'::character varying, 'CANCELLED'::character varying])::text[])))
);


--
-- Name: requisition_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.requisition_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: requisition_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.requisition_id_seq OWNED BY public.requisition.id;


--
-- Name: requisition_line; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.requisition_line (
    id bigint NOT NULL,
    req_id bigint NOT NULL,
    line_no integer,
    stock_item_id bigint,
    mat_code character varying(20) NOT NULL,
    unit character varying(50),
    qty_requested numeric(18,4) NOT NULL,
    qty_issued numeric(18,4) DEFAULT 0 NOT NULL,
    unit_cost numeric(18,4),
    total_cost numeric(18,4),
    remarks text
);


--
-- Name: requisition_line_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.requisition_line_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: requisition_line_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.requisition_line_id_seq OWNED BY public.requisition_line.id;


--
-- Name: requisition_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.requisition_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: requisition_status_log; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.requisition_status_log (
    id bigint NOT NULL,
    req_id bigint NOT NULL,
    from_status character varying(30),
    to_status character varying(30) NOT NULL,
    changed_by bigint NOT NULL,
    changed_at timestamp without time zone DEFAULT now() NOT NULL,
    remarks text
);


--
-- Name: requisition_status_log_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.requisition_status_log_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: requisition_status_log_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.requisition_status_log_id_seq OWNED BY public.requisition_status_log.id;


--
-- Name: ret_number_counter; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ret_number_counter (
    year_month character varying NOT NULL,
    last_seq integer DEFAULT 0 NOT NULL
);


--
-- Name: rfq; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rfq (
    id bigint NOT NULL,
    rfq_no character varying(30) NOT NULL,
    rfq_date date DEFAULT CURRENT_DATE NOT NULL,
    pr_id bigint,
    status character varying(20) DEFAULT 'SENT'::character varying NOT NULL,
    remarks text,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint NOT NULL,
    updated_by bigint,
    supplier_id integer,
    CONSTRAINT rfq_status_check CHECK (((status)::text = ANY (ARRAY['SENT'::text, 'RECEIVED'::text, 'SELECTED'::text, 'REJECTED'::text])))
);


--
-- Name: rfq_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.rfq ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.rfq_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: rfq_line; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rfq_line (
    id bigint NOT NULL,
    rfq_id bigint NOT NULL,
    line_no integer NOT NULL,
    mat_code character varying(20) NOT NULL,
    qty numeric(18,4) NOT NULL,
    unit_price numeric(18,4),
    currency character varying(10) DEFAULT 'THB'::character varying,
    lead_time_days integer,
    remarks text
);


--
-- Name: rfq_line_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.rfq_line ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.rfq_line_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: role_menu_permissions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.role_menu_permissions (
    id bigint NOT NULL,
    role_id bigint NOT NULL,
    menu_id bigint NOT NULL,
    can_read boolean DEFAULT false NOT NULL,
    can_write boolean DEFAULT false NOT NULL,
    can_update boolean DEFAULT false NOT NULL,
    can_delete boolean DEFAULT false NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint
);


--
-- Name: role_menu_permissions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.role_menu_permissions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: role_menu_permissions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.role_menu_permissions_id_seq OWNED BY public.role_menu_permissions.id;


--
-- Name: role_menus; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.role_menus (
    role_id bigint NOT NULL,
    menu_id bigint NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint
);


--
-- Name: role_permissions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.role_permissions (
    role_id bigint NOT NULL,
    permission_id bigint NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint
);


--
-- Name: roles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.roles (
    id bigint NOT NULL,
    role_code character varying(100) NOT NULL,
    role_name character varying(255) NOT NULL,
    description text,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint,
    is_active boolean DEFAULT true NOT NULL,
    level integer DEFAULT 0 NOT NULL,
    department character varying(100),
    dept_code character varying(50)
);


--
-- Name: roles_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.roles ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.roles_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: schema_migrations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.schema_migrations (
    version text NOT NULL,
    applied_at timestamp with time zone DEFAULT now() NOT NULL,
    baseline boolean DEFAULT false NOT NULL
);


--
-- Name: spec_size; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.spec_size (
    id integer NOT NULL,
    spec_code character varying(10) NOT NULL,
    mat_name_id integer NOT NULL,
    spec_description character varying(200) NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint
);


--
-- Name: spec_size_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.spec_size_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: spec_size_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.spec_size_id_seq OWNED BY public.spec_size.id;


--
-- Name: stock_category; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.stock_category (
    id bigint NOT NULL,
    code character varying(20) NOT NULL,
    name character varying(100) NOT NULL,
    description text,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL
);


--
-- Name: TABLE stock_category; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.stock_category IS 'หมวดหมู่ stock item — แยกจาก mat_group ของ PR/PO';


--
-- Name: stock_category_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.stock_category ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.stock_category_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: stock_count; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.stock_count (
    id bigint NOT NULL,
    count_no character varying(30) NOT NULL,
    warehouse_code character varying(20) NOT NULL,
    count_date date NOT NULL,
    status character varying(20) DEFAULT 'DRAFT'::character varying NOT NULL,
    remarks text,
    approved_by bigint,
    approved_at timestamp without time zone,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint NOT NULL,
    updated_by bigint,
    CONSTRAINT stock_count_status_check CHECK (((status)::text = ANY (ARRAY['DRAFT'::text, 'IN_PROGRESS'::text, 'COMPLETED'::text])))
);


--
-- Name: stock_count_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.stock_count ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.stock_count_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: stock_count_line; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.stock_count_line (
    id bigint NOT NULL,
    count_id bigint NOT NULL,
    mat_code character varying(20) NOT NULL,
    zone_id integer,
    qty_system numeric(18,4),
    qty_counted numeric(18,4),
    qty_diff numeric(18,4) GENERATED ALWAYS AS ((qty_counted - qty_system)) STORED,
    remarks text
);


--
-- Name: stock_count_line_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.stock_count_line ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.stock_count_line_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: stock_inventory; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.stock_inventory (
    id bigint NOT NULL,
    item_id bigint NOT NULL,
    location_code character varying(20) NOT NULL,
    warehouse_code character varying(20) NOT NULL,
    qty_on_hand numeric(18,4) DEFAULT 0 NOT NULL,
    qty_reserved numeric(18,4) DEFAULT 0 NOT NULL,
    qty_available numeric(18,4) GENERATED ALWAYS AS ((qty_on_hand - qty_reserved)) STORED,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    CONSTRAINT stock_inventory_qty_check CHECK ((qty_on_hand >= (0)::numeric)),
    CONSTRAINT stock_inventory_res_check CHECK ((qty_reserved >= (0)::numeric))
);


--
-- Name: TABLE stock_inventory; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.stock_inventory IS 'Stock balance per item per location';


--
-- Name: COLUMN stock_inventory.qty_available; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.stock_inventory.qty_available IS 'qty_on_hand - qty_reserved (generated)';


--
-- Name: stock_inventory_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.stock_inventory ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.stock_inventory_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: stock_item; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.stock_item (
    id bigint NOT NULL,
    mat_code character varying(30) CONSTRAINT stock_item_item_code_not_null NOT NULL,
    item_name character varying(255) NOT NULL,
    description text,
    category_id bigint,
    item_type character varying(20) DEFAULT 'RETURNABLE'::character varying NOT NULL,
    tracking_type character varying(10) DEFAULT 'sku'::character varying NOT NULL,
    unit character varying(50) NOT NULL,
    qty numeric(18,4) DEFAULT 0 CONSTRAINT stock_item_min_qty_not_null NOT NULL,
    qr_code text,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint,
    unit_cost numeric(18,4) DEFAULT 0 NOT NULL,
    warehouse_code character varying(20) DEFAULT 'WH01'::character varying NOT NULL,
    location_code character varying(20) DEFAULT 'SAL'::character varying NOT NULL,
    description_store text,
    CONSTRAINT stock_item_min_qty_check CHECK ((qty >= (0)::numeric)),
    CONSTRAINT stock_item_tracking_check CHECK (((tracking_type)::text = ANY (ARRAY['sku'::text, 'serial'::text]))),
    CONSTRAINT stock_item_type_check CHECK (((item_type)::text = ANY (ARRAY['RETURNABLE'::text, 'CONSUMABLE'::text]))),
    CONSTRAINT stock_item_unit_cost_check CHECK ((unit_cost >= (0)::numeric))
);


--
-- Name: TABLE stock_item; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.stock_item IS 'Stock item master — คนละตัวกับ material_code ของ PR/PO';


--
-- Name: COLUMN stock_item.mat_code; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.stock_item.mat_code IS 'ใช้ค่าเดียวกับ mat_code ฝั่ง Material master (PR/PO) แต่เป็นคนละตาราง
   ไม่มี FK ข้าม schema — ระวังตอน query/join ว่ากำลังอ้างถึง mat_code
   ฝั่ง Stock หรือฝั่ง Material';


--
-- Name: COLUMN stock_item.item_type; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.stock_item.item_type IS 'RETURNABLE=ต้องคืน, CONSUMABLE=ไม่ต้องคืน';


--
-- Name: COLUMN stock_item.tracking_type; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.stock_item.tracking_type IS 'sku=per SKU (ปัจจุบัน), serial=per unit (อนาคต)';


--
-- Name: COLUMN stock_item.qty; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.stock_item.qty IS 'จำนวนคงเหลือ (qty on hand) — เก็บตรงบน stock_item ไปก่อน ยังไม่แยกตาม
   location เต็มรูปแบบ ดู warehouse_code/location_code คู่กันสำหรับรู้ว่า
   อยู่คลังไหน จะเชื่อมกับระบบ multi-location stock_inventory ทีหลัง';


--
-- Name: COLUMN stock_item.qr_code; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.stock_item.qr_code IS 'QR code value — default = item_code';


--
-- Name: COLUMN stock_item.unit_cost; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.stock_item.unit_cost IS 'ต้นทุนต่อหน่วย (บาท) — จาก column "UNIT" ใต้หัว "MATERIAL COST" ในไฟล์
   Excel ต้นทาง ใช้คำนวณ amount = qty * unit_cost ตอนแสดงผล ไม่ต้อง
   เก็บ amount แยกในตารางนี้';


--
-- Name: stock_item_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.stock_item ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.stock_item_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: stock_item_image; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.stock_item_image (
    id bigint NOT NULL,
    item_id bigint NOT NULL,
    file_path text NOT NULL,
    file_name character varying(255),
    is_primary boolean DEFAULT false NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint
);


--
-- Name: stock_item_image_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.stock_item_image_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: stock_item_image_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.stock_item_image_id_seq OWNED BY public.stock_item_image.id;


--
-- Name: stock_item_qty_backup_20261011; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.stock_item_qty_backup_20261011 (
    id bigint,
    mat_code character varying(30),
    item_name character varying(255),
    description text,
    category_id bigint,
    item_type character varying(20),
    tracking_type character varying(10),
    unit character varying(50),
    qty numeric(18,4),
    qr_code text,
    is_active boolean,
    created_at timestamp without time zone,
    updated_at timestamp without time zone,
    created_by bigint,
    updated_by bigint,
    unit_cost numeric(18,4),
    warehouse_code character varying(20),
    location_code character varying(20),
    description_store text
);


--
-- Name: stock_item_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.stock_item_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: stock_res_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.stock_res_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: stock_reservation; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.stock_reservation (
    id bigint NOT NULL,
    reservation_no character varying(30) NOT NULL,
    item_id bigint NOT NULL,
    location_code character varying(20) NOT NULL,
    qty_reserved numeric(18,4) NOT NULL,
    qty_fulfilled numeric(18,4) DEFAULT 0 NOT NULL,
    status character varying(20) DEFAULT 'PENDING'::character varying NOT NULL,
    requested_by bigint NOT NULL,
    needed_by date,
    purpose text,
    ref_doc_type character varying(20),
    ref_doc_id bigint,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    cancelled_at timestamp without time zone,
    CONSTRAINT stock_res_qty_check CHECK ((qty_reserved > (0)::numeric)),
    CONSTRAINT stock_res_status_check CHECK (((status)::text = ANY (ARRAY['PENDING'::text, 'CONFIRMED'::text, 'CANCELLED'::text, 'FULFILLED'::text])))
);


--
-- Name: TABLE stock_reservation; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.stock_reservation IS 'จองวัสดุล่วงหน้า ป้องกัน over-commit';


--
-- Name: stock_reservation_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.stock_reservation ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.stock_reservation_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: stock_transaction; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.stock_transaction (
    id bigint NOT NULL,
    txn_no character varying(30) NOT NULL,
    txn_type character varying(20) NOT NULL,
    item_id bigint NOT NULL,
    from_location character varying(20),
    to_location character varying(20),
    qty numeric(18,4) NOT NULL,
    ref_doc_type character varying(20),
    ref_doc_id bigint,
    remarks text,
    txn_date date DEFAULT CURRENT_DATE NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint NOT NULL,
    qty_before numeric(18,4),
    qty_after numeric(18,4),
    reversed_at timestamp without time zone,
    receive_document_id bigint,
    return_document_id bigint,
    CONSTRAINT stock_txn_qty_check CHECK ((qty > (0)::numeric)),
    CONSTRAINT stock_txn_type_check CHECK (((txn_type)::text = ANY (ARRAY['IN'::text, 'OUT'::text, 'TRANSFER'::text, 'ADJUST_PLUS'::text, 'ADJUST_MINUS'::text, 'BORROW_OUT'::text, 'BORROW_RETURN'::text])))
);


--
-- Name: COLUMN stock_transaction.qty_before; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.stock_transaction.qty_before IS 'ยอดคงเหลือก่อนรายการนี้ (snapshot ตอน insert เท่านั้น ไม่ update ย้อนหลัง)';


--
-- Name: COLUMN stock_transaction.qty_after; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.stock_transaction.qty_after IS 'ยอดคงเหลือหลังรายการนี้';


--
-- Name: stock_transaction_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.stock_transaction ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.stock_transaction_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: stock_transfer; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.stock_transfer (
    id bigint NOT NULL,
    transfer_no character varying(30) NOT NULL,
    transfer_type character varying(20) NOT NULL,
    transfer_date date NOT NULL,
    from_warehouse_code character varying(20),
    from_project_code character varying(20),
    to_warehouse_code character varying(20),
    to_project_code character varying(20),
    requested_by bigint NOT NULL,
    purpose text,
    remarks text,
    status character varying(20) DEFAULT 'DRAFT'::character varying NOT NULL,
    checked_by bigint,
    checked_at timestamp without time zone,
    created_by bigint NOT NULL,
    updated_by bigint,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL
);


--
-- Name: stock_transfer_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.stock_transfer_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: stock_transfer_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.stock_transfer_id_seq OWNED BY public.stock_transfer.id;


--
-- Name: stock_transfer_line; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.stock_transfer_line (
    id bigint NOT NULL,
    transfer_id bigint NOT NULL,
    line_no integer NOT NULL,
    item_id bigint NOT NULL,
    mat_code character varying(30) NOT NULL,
    unit character varying(50),
    qty_requested numeric NOT NULL,
    qty_confirmed numeric,
    remarks text
);


--
-- Name: stock_transfer_line_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.stock_transfer_line_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: stock_transfer_line_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.stock_transfer_line_id_seq OWNED BY public.stock_transfer_line.id;


--
-- Name: stock_transfer_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.stock_transfer_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: stock_transfer_status_log; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.stock_transfer_status_log (
    id bigint NOT NULL,
    transfer_id bigint NOT NULL,
    from_status character varying(20),
    to_status character varying(20) NOT NULL,
    changed_by bigint NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL
);


--
-- Name: stock_transfer_status_log_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.stock_transfer_status_log_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: stock_transfer_status_log_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.stock_transfer_status_log_id_seq OWNED BY public.stock_transfer_status_log.id;


--
-- Name: stock_txn_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.stock_txn_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: storage_zone; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.storage_zone (
    id integer NOT NULL,
    warehouse_id integer NOT NULL,
    zone_code character varying(30) NOT NULL,
    zone_name character varying(100) NOT NULL,
    zone_type character varying(50),
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint,
    CONSTRAINT storage_zone_type_check CHECK (((zone_type)::text = ANY (ARRAY['rack'::text, 'shelf'::text, 'bin'::text, 'area'::text])))
);


--
-- Name: storage_zone_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.storage_zone_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: storage_zone_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.storage_zone_id_seq OWNED BY public.storage_zone.id;


--
-- Name: subgroup; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.subgroup (
    id integer NOT NULL,
    subgroup_code character varying(10) NOT NULL,
    group_id integer NOT NULL,
    subgroup_name character varying(100) NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint
);


--
-- Name: subgroup_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.subgroup_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: subgroup_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.subgroup_id_seq OWNED BY public.subgroup.id;


--
-- Name: supplier; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.supplier (
    id integer NOT NULL,
    supplier_name character varying(200) NOT NULL,
    tax_id character varying(50),
    address text,
    contact_name character varying(200),
    contact_phone character varying(50),
    contact_email character varying(200),
    payment_terms character varying(100),
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint,
    office_phone character varying(50),
    fax character varying(50),
    currency character varying(10) DEFAULT 'BH'::character varying,
    sales_person character varying,
    sales_person_phone character varying(50),
    remarks text,
    CONSTRAINT supplier_currency_check CHECK (((currency)::text = ANY ((ARRAY['BH'::character varying, 'US'::character varying])::text[])))
);


--
-- Name: COLUMN supplier.office_phone; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.supplier.office_phone IS 'เบอร์โทรศัพท์สำนักงาน';


--
-- Name: COLUMN supplier.fax; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.supplier.fax IS 'หมายเลขแฟกซ์';


--
-- Name: COLUMN supplier.currency; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.supplier.currency IS 'สกุลเงิน: BH = บาท, US = ดอลลาร์';


--
-- Name: supplier_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.supplier_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: supplier_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.supplier_id_seq OWNED BY public.supplier.id;


--
-- Name: unit; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.unit (
    id integer NOT NULL,
    unit_code character varying(10) NOT NULL,
    unit_name character varying(50) NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint
);


--
-- Name: unit_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.unit_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: unit_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.unit_id_seq OWNED BY public.unit.id;


--
-- Name: user_menu_permissions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_menu_permissions (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    menu_id bigint NOT NULL,
    can_read boolean DEFAULT false NOT NULL,
    can_write boolean DEFAULT false NOT NULL,
    can_update boolean DEFAULT false NOT NULL,
    can_delete boolean DEFAULT false NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint
);


--
-- Name: user_menu_permissions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.user_menu_permissions ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.user_menu_permissions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: user_permissions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_permissions (
    user_id bigint NOT NULL,
    permission_id bigint NOT NULL,
    is_allow boolean NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint
);


--
-- Name: user_roles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_roles (
    user_id bigint NOT NULL,
    role_id bigint NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint
);


--
-- Name: users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.users (
    id bigint NOT NULL,
    username character varying(100) NOT NULL,
    full_name character varying(255) NOT NULL,
    email character varying(255),
    password_hash text,
    is_active boolean DEFAULT true NOT NULL,
    location_code character varying(20),
    employee_code character varying(50),
    department character varying(200),
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint,
    dept_code character varying(50),
    signature_path text,
    signature_mime character varying(50),
    signature_updated_at timestamp without time zone,
    full_name_th character varying(255),
    phone character varying(30)
);


--
-- Name: COLUMN users.full_name_th; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.users.full_name_th IS 'ชื่อ-นามสกุลภาษาไทย (optional); full_name เดิมเก็บชื่อภาษาอังกฤษ';


--
-- Name: users_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.users ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.users_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: warehouse; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.warehouse (
    id integer NOT NULL,
    warehouse_code character varying(20) NOT NULL,
    warehouse_name character varying(200) NOT NULL,
    address text,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint
);


--
-- Name: warehouse_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.warehouse_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: warehouse_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.warehouse_id_seq OWNED BY public.warehouse.id;


--
-- Name: wht_income_type; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.wht_income_type (
    id integer NOT NULL,
    code character varying(30) NOT NULL,
    label_th character varying(150) NOT NULL,
    rate numeric(5,2) NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT wht_income_type_rate_check CHECK (((rate > (0)::numeric) AND (rate <= (15)::numeric)))
);


--
-- Name: wht_income_type_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.wht_income_type ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.wht_income_type_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: wo_contractor_log; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.wo_contractor_log (
    id bigint NOT NULL,
    wo_id bigint NOT NULL,
    project_code character varying(20),
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint
);


--
-- Name: wo_contractor_log_bill; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.wo_contractor_log_bill (
    id bigint NOT NULL,
    log_id bigint NOT NULL,
    bill_type character varying(20) NOT NULL,
    installment_no integer,
    ap_voucher_no character varying(50),
    pay_to character varying(200),
    credit_term_days integer,
    description text,
    progress_installment_no integer,
    wht_type character varying(50),
    subtotal numeric(18,2) DEFAULT 0 NOT NULL,
    net_amount numeric(18,2) DEFAULT 0 NOT NULL,
    status character varying(20) DEFAULT 'DRAFT'::character varying NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint,
    CONSTRAINT wo_contractor_log_bill_bill_type_check CHECK (((bill_type)::text = ANY ((ARRAY['ADV'::character varying, 'PROGRESS'::character varying, 'RETENTION'::character varying])::text[]))),
    CONSTRAINT wo_contractor_log_bill_status_check CHECK (((status)::text = ANY ((ARRAY['DRAFT'::character varying, 'SAVED'::character varying])::text[])))
);


--
-- Name: wo_contractor_log_bill_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.wo_contractor_log_bill ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.wo_contractor_log_bill_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: wo_contractor_log_bill_invoice; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.wo_contractor_log_bill_invoice (
    id bigint NOT NULL,
    bill_id bigint NOT NULL,
    line_no integer NOT NULL,
    invoice_no character varying(50),
    invoice_date date,
    delivery_date date,
    bill_date date,
    pay_date date
);


--
-- Name: wo_contractor_log_bill_invoice_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.wo_contractor_log_bill_invoice ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.wo_contractor_log_bill_invoice_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: wo_contractor_log_bill_line; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.wo_contractor_log_bill_line (
    id bigint NOT NULL,
    bill_id bigint NOT NULL,
    line_code character varying(20) NOT NULL,
    percent numeric(7,2),
    amount numeric(18,2) DEFAULT 0 NOT NULL,
    CONSTRAINT wo_contractor_log_bill_line_line_code_check CHECK (((line_code)::text = ANY ((ARRAY['ADV'::character varying, 'PROGRESS'::character varying, 'ADV_DEDUCT'::character varying, 'VAT'::character varying, 'RETENTION'::character varying, 'WHT'::character varying, 'OTHER'::character varying])::text[])))
);


--
-- Name: wo_contractor_log_bill_line_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.wo_contractor_log_bill_line ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.wo_contractor_log_bill_line_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: wo_contractor_log_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.wo_contractor_log ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.wo_contractor_log_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: wo_contractor_log_term; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.wo_contractor_log_term (
    id bigint NOT NULL,
    log_id bigint NOT NULL,
    item_type character varying(20) NOT NULL,
    percent numeric(7,2),
    amount numeric(18,2),
    accumulate numeric(18,2),
    balance numeric(18,2),
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    CONSTRAINT wo_contractor_log_term_item_type_check CHECK (((item_type)::text = ANY ((ARRAY['CONTRACT'::character varying, 'ADVANCE'::character varying, 'RETENTION'::character varying])::text[])))
);


--
-- Name: wo_contractor_log_term_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.wo_contractor_log_term ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.wo_contractor_log_term_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: wo_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.wo_seq
    START WITH 2
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: work_order; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.work_order (
    id bigint NOT NULL,
    wo_no character varying(30) NOT NULL,
    wo_date date DEFAULT CURRENT_DATE NOT NULL,
    employer_name character varying(200) NOT NULL,
    project_code character varying(20),
    project_scope_text text,
    supplier_name character varying(200) NOT NULL,
    contact_person character varying(100),
    supplier_address text,
    supplier_phone character varying(50),
    contract_type character varying(20) DEFAULT 'LABOR_MATERIAL'::character varying NOT NULL,
    work_system text[] NOT NULL,
    contract_description text,
    contract_amount numeric(18,2) DEFAULT 0 NOT NULL,
    vat_rate numeric(5,2) DEFAULT 7.00 NOT NULL,
    wht_rate numeric(5,2) DEFAULT 3.00 NOT NULL,
    advance_pct numeric(5,2) DEFAULT 0 NOT NULL,
    advance_amount numeric(18,2) DEFAULT 0 NOT NULL,
    progress_payment_note text,
    retention_pct numeric(5,2) DEFAULT 5.00 NOT NULL,
    advance_deduct_pct numeric(5,2) DEFAULT 0 NOT NULL,
    other_deduction_note text,
    start_date date,
    duration_days integer,
    end_date date,
    penalty_pct_per_day numeric(5,2) DEFAULT 0 NOT NULL,
    warranty_years integer DEFAULT 1 NOT NULL,
    ref_no character varying(50),
    other_terms text,
    cost_code character varying(50),
    status character varying(20) DEFAULT 'DRAFT'::character varying NOT NULL,
    entered_by bigint,
    entered_at timestamp without time zone,
    section_head_id bigint,
    section_head_signed_at timestamp without time zone,
    authorized_by bigint,
    authorized_at timestamp without time zone,
    subcontractor_signed_name character varying(200),
    subcontractor_signed_at timestamp without time zone,
    remarks text,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint NOT NULL,
    updated_by bigint,
    use_discount boolean DEFAULT false NOT NULL,
    discount_type character varying(10) DEFAULT 'pct'::character varying NOT NULL,
    use_vat boolean DEFAULT false NOT NULL,
    use_wht boolean DEFAULT false NOT NULL,
    total_amount numeric(18,4) DEFAULT 0 NOT NULL,
    discount_amount numeric(18,4) DEFAULT 0 NOT NULL,
    vat_amount numeric(18,4) DEFAULT 0 NOT NULL,
    wht_amount numeric(18,4) DEFAULT 0 NOT NULL,
    net_amount numeric(18,4) DEFAULT 0 NOT NULL,
    supplier_id integer,
    project_name_text character varying(200),
    advance_deduct_amount numeric(18,2) DEFAULT 0 NOT NULL,
    approver_id bigint,
    has_penalty boolean DEFAULT false NOT NULL,
    warranty_note text,
    CONSTRAINT chk_wo_contract_type CHECK (((contract_type)::text = ANY ((ARRAY['LABOR_MATERIAL'::character varying, 'LABOR_ONLY'::character varying, 'RENTAL'::character varying, 'SERVICE'::character varying])::text[]))),
    CONSTRAINT chk_wo_status CHECK (((status)::text = ANY (ARRAY['DRAFT'::text, 'PENDING_APPROVAL'::text, 'APPROVED'::text, 'REJECTED'::text, 'PENDING_REAPPROVAL'::text, 'CANCELLED'::text]))),
    CONSTRAINT chk_wo_work_system CHECK (((cardinality(work_system) >= 1) AND (work_system <@ ARRAY['P'::text, 'E'::text, 'S'::text, 'MP'::text, 'ME'::text, 'MS'::text, 'MF'::text, 'MG'::text, 'MH'::text]))),
    CONSTRAINT wo_discount_type_check CHECK (((discount_type)::text = ANY ((ARRAY['pct'::character varying, 'amt'::character varying])::text[])))
);


--
-- Name: work_order_attachment; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.work_order_attachment (
    id bigint NOT NULL,
    wo_id bigint NOT NULL,
    file_name character varying(255) NOT NULL,
    file_path character varying(500) NOT NULL,
    file_size bigint,
    file_type character varying(50),
    uploaded_at timestamp without time zone DEFAULT now() NOT NULL,
    uploaded_by bigint NOT NULL
);


--
-- Name: work_order_attachment_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.work_order_attachment ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.work_order_attachment_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: work_order_cost_code; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.work_order_cost_code (
    id bigint NOT NULL,
    wo_id bigint NOT NULL,
    cost_code character varying(50) NOT NULL,
    remarks character varying(255),
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint NOT NULL
);


--
-- Name: TABLE work_order_cost_code; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.work_order_cost_code IS 'DEPRECATED as of the WO line-item rework — superseded by work_order_line. Not dropped yet; confirm before removing.';


--
-- Name: work_order_cost_code_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.work_order_cost_code ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.work_order_cost_code_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: work_order_edit_log; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.work_order_edit_log (
    id bigint NOT NULL,
    wo_id bigint NOT NULL,
    edited_by bigint NOT NULL,
    reason text NOT NULL,
    edited_at timestamp without time zone DEFAULT now() NOT NULL
);


--
-- Name: work_order_edit_log_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.work_order_edit_log ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.work_order_edit_log_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: work_order_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.work_order ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.work_order_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: work_order_line; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.work_order_line (
    id bigint NOT NULL,
    wo_id bigint NOT NULL,
    sort_order integer NOT NULL,
    cost_code character varying(50) NOT NULL,
    description text,
    qty numeric(18,4) NOT NULL,
    unit_price numeric(18,4) NOT NULL,
    amount numeric(18,4) GENERATED ALWAYS AS ((qty * unit_price)) STORED,
    disc numeric(18,4) DEFAULT 0 NOT NULL,
    disc_type character varying(10) DEFAULT 'pct'::character varying NOT NULL,
    vat_rate numeric(5,2),
    wht_rate numeric(5,2),
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint NOT NULL,
    CONSTRAINT chk_wol_wht_rate CHECK (((wht_rate IS NULL) OR (wht_rate = ANY (ARRAY[(1)::numeric, (3)::numeric, (5)::numeric])))),
    CONSTRAINT work_order_line_disc_type_check CHECK (((disc_type)::text = ANY ((ARRAY['pct'::character varying, 'amt'::character varying])::text[]))),
    CONSTRAINT work_order_line_qty_check CHECK ((qty > (0)::numeric)),
    CONSTRAINT work_order_line_unit_price_check CHECK ((unit_price >= (0)::numeric)),
    CONSTRAINT work_order_line_wht_rate_check CHECK (((wht_rate IS NULL) OR (wht_rate = ANY (ARRAY[(1)::numeric, (3)::numeric, (5)::numeric]))))
);


--
-- Name: work_order_line_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.work_order_line ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.work_order_line_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: work_order_payment_installment; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.work_order_payment_installment (
    id bigint NOT NULL,
    wo_id bigint NOT NULL,
    installment_no integer NOT NULL,
    description text,
    percent_of_contract numeric(5,2),
    amount numeric(18,2) DEFAULT 0 NOT NULL,
    due_date date,
    payment_status character varying(20) DEFAULT 'UNPAID'::character varying NOT NULL,
    paid_date date,
    remarks text,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint
);


--
-- Name: work_order_payment_installment_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.work_order_payment_installment_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: work_order_payment_installment_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.work_order_payment_installment_id_seq OWNED BY public.work_order_payment_installment.id;


--
-- Name: work_order_penalty; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.work_order_penalty (
    id bigint NOT NULL,
    wo_id bigint NOT NULL,
    description text,
    percent_per_day numeric(5,2),
    contract_start_date date,
    contract_end_date date,
    remarks text,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint
);


--
-- Name: work_order_penalty_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.work_order_penalty_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: work_order_penalty_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.work_order_penalty_id_seq OWNED BY public.work_order_penalty.id;


--
-- Name: work_order_retention; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.work_order_retention (
    id bigint NOT NULL,
    wo_id bigint NOT NULL,
    description text,
    percent_of_contract numeric(5,2),
    amount numeric(18,2) DEFAULT 0 NOT NULL,
    remarks text,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_by bigint,
    updated_by bigint
);


--
-- Name: work_order_retention_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.work_order_retention_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: work_order_retention_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.work_order_retention_id_seq OWNED BY public.work_order_retention.id;


--
-- Name: work_order_status_log; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.work_order_status_log (
    id bigint NOT NULL,
    wo_id bigint NOT NULL,
    from_status character varying(20),
    to_status character varying(20) NOT NULL,
    changed_by bigint NOT NULL,
    changed_at timestamp without time zone DEFAULT now() NOT NULL,
    remarks text
);


--
-- Name: work_order_status_log_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.work_order_status_log ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.work_order_status_log_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: brand id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.brand ALTER COLUMN id SET DEFAULT nextval('public.brand_id_seq'::regclass);


--
-- Name: cost_group id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cost_group ALTER COLUMN id SET DEFAULT nextval('public.cost_group_id_seq'::regclass);


--
-- Name: cost_job id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cost_job ALTER COLUMN id SET DEFAULT nextval('public.cost_job_id_seq'::regclass);


--
-- Name: cost_subgroup id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cost_subgroup ALTER COLUMN id SET DEFAULT nextval('public.cost_subgroup_id_seq'::regclass);


--
-- Name: cost_subject id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cost_subject ALTER COLUMN id SET DEFAULT nextval('public.cost_subject_id_seq'::regclass);


--
-- Name: customer cus_id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.customer ALTER COLUMN cus_id SET DEFAULT nextval('public.customer_cus_id_seq'::regclass);


--
-- Name: ic_po_receive_document id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_po_receive_document ALTER COLUMN id SET DEFAULT nextval('public.ic_po_receive_document_id_seq'::regclass);


--
-- Name: ic_project_cost_item id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_project_cost_item ALTER COLUMN id SET DEFAULT nextval('public.ic_project_cost_item_id_seq'::regclass);


--
-- Name: ic_project_cost_item_transaction id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_project_cost_item_transaction ALTER COLUMN id SET DEFAULT nextval('public.ic_project_cost_item_transaction_id_seq'::regclass);


--
-- Name: location id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.location ALTER COLUMN id SET DEFAULT nextval('public.location_id_seq'::regclass);


--
-- Name: mat_group id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mat_group ALTER COLUMN id SET DEFAULT nextval('public.mat_group_id_seq'::regclass);


--
-- Name: mat_name id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mat_name ALTER COLUMN id SET DEFAULT nextval('public.mat_name_id_seq'::regclass);


--
-- Name: material_code id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.material_code ALTER COLUMN id SET DEFAULT nextval('public.material_code_id_seq'::regclass);


--
-- Name: pr_attachment id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pr_attachment ALTER COLUMN id SET DEFAULT nextval('public.pr_attachment_id_seq'::regclass);


--
-- Name: project id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.project ALTER COLUMN id SET DEFAULT nextval('public.project_id_seq'::regclass);


--
-- Name: requisition id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.requisition ALTER COLUMN id SET DEFAULT nextval('public.requisition_id_seq'::regclass);


--
-- Name: requisition_line id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.requisition_line ALTER COLUMN id SET DEFAULT nextval('public.requisition_line_id_seq'::regclass);


--
-- Name: requisition_status_log id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.requisition_status_log ALTER COLUMN id SET DEFAULT nextval('public.requisition_status_log_id_seq'::regclass);


--
-- Name: role_menu_permissions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.role_menu_permissions ALTER COLUMN id SET DEFAULT nextval('public.role_menu_permissions_id_seq'::regclass);


--
-- Name: spec_size id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.spec_size ALTER COLUMN id SET DEFAULT nextval('public.spec_size_id_seq'::regclass);


--
-- Name: stock_item_image id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_item_image ALTER COLUMN id SET DEFAULT nextval('public.stock_item_image_id_seq'::regclass);


--
-- Name: stock_transfer id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_transfer ALTER COLUMN id SET DEFAULT nextval('public.stock_transfer_id_seq'::regclass);


--
-- Name: stock_transfer_line id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_transfer_line ALTER COLUMN id SET DEFAULT nextval('public.stock_transfer_line_id_seq'::regclass);


--
-- Name: stock_transfer_status_log id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_transfer_status_log ALTER COLUMN id SET DEFAULT nextval('public.stock_transfer_status_log_id_seq'::regclass);


--
-- Name: storage_zone id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.storage_zone ALTER COLUMN id SET DEFAULT nextval('public.storage_zone_id_seq'::regclass);


--
-- Name: subgroup id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subgroup ALTER COLUMN id SET DEFAULT nextval('public.subgroup_id_seq'::regclass);


--
-- Name: supplier id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.supplier ALTER COLUMN id SET DEFAULT nextval('public.supplier_id_seq'::regclass);


--
-- Name: unit id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.unit ALTER COLUMN id SET DEFAULT nextval('public.unit_id_seq'::regclass);


--
-- Name: warehouse id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.warehouse ALTER COLUMN id SET DEFAULT nextval('public.warehouse_id_seq'::regclass);


--
-- Name: work_order_payment_installment id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.work_order_payment_installment ALTER COLUMN id SET DEFAULT nextval('public.work_order_payment_installment_id_seq'::regclass);


--
-- Name: work_order_penalty id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.work_order_penalty ALTER COLUMN id SET DEFAULT nextval('public.work_order_penalty_id_seq'::regclass);


--
-- Name: work_order_retention id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.work_order_retention ALTER COLUMN id SET DEFAULT nextval('public.work_order_retention_id_seq'::regclass);


--
-- Name: approval_config approval_config_doc_type_step_no_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_config
    ADD CONSTRAINT approval_config_doc_type_step_no_key UNIQUE (doc_type, step_no);


--
-- Name: approval_config approval_config_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_config
    ADD CONSTRAINT approval_config_pkey PRIMARY KEY (id);


--
-- Name: approval_delegation approval_delegation_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_delegation
    ADD CONSTRAINT approval_delegation_pkey PRIMARY KEY (id);


--
-- Name: approval_doc_types approval_doc_types_doc_type_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_doc_types
    ADD CONSTRAINT approval_doc_types_doc_type_key UNIQUE (doc_type);


--
-- Name: approval_doc_types approval_doc_types_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_doc_types
    ADD CONSTRAINT approval_doc_types_pkey PRIMARY KEY (id);


--
-- Name: approval_log approval_log_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_log
    ADD CONSTRAINT approval_log_pkey PRIMARY KEY (id);


--
-- Name: approval_request approval_request_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_request
    ADD CONSTRAINT approval_request_pkey PRIMARY KEY (id);


--
-- Name: borrow borrow_borrow_no_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.borrow
    ADD CONSTRAINT borrow_borrow_no_key UNIQUE (borrow_no);


--
-- Name: borrow_line borrow_line_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.borrow_line
    ADD CONSTRAINT borrow_line_pkey PRIMARY KEY (id);


--
-- Name: borrow borrow_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.borrow
    ADD CONSTRAINT borrow_pkey PRIMARY KEY (id);


--
-- Name: borrow_status_log borrow_status_log_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.borrow_status_log
    ADD CONSTRAINT borrow_status_log_pkey PRIMARY KEY (id);


--
-- Name: brand brand_brand_code_spec_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.brand
    ADD CONSTRAINT brand_brand_code_spec_id_key UNIQUE (brand_code, spec_id);


--
-- Name: brand brand_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.brand
    ADD CONSTRAINT brand_pkey PRIMARY KEY (id);


--
-- Name: cost_group cost_group_job_id_group_code_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cost_group
    ADD CONSTRAINT cost_group_job_id_group_code_key UNIQUE (job_id, group_code);


--
-- Name: cost_group cost_group_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cost_group
    ADD CONSTRAINT cost_group_pkey PRIMARY KEY (id);


--
-- Name: cost_job cost_job_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cost_job
    ADD CONSTRAINT cost_job_pkey PRIMARY KEY (id);


--
-- Name: cost_job cost_job_subject_id_job_code_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cost_job
    ADD CONSTRAINT cost_job_subject_id_job_code_key UNIQUE (subject_id, job_code);


--
-- Name: cost_subgroup cost_subgroup_group_id_subgroup_code_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cost_subgroup
    ADD CONSTRAINT cost_subgroup_group_id_subgroup_code_key UNIQUE (group_id, subgroup_code);


--
-- Name: cost_subgroup cost_subgroup_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cost_subgroup
    ADD CONSTRAINT cost_subgroup_pkey PRIMARY KEY (id);


--
-- Name: cost_subject cost_subject_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cost_subject
    ADD CONSTRAINT cost_subject_pkey PRIMARY KEY (id);


--
-- Name: cost_subject cost_subject_subject_code_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cost_subject
    ADD CONSTRAINT cost_subject_subject_code_key UNIQUE (subject_code);


--
-- Name: customer customer_customer_code_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.customer
    ADD CONSTRAINT customer_customer_code_key UNIQUE (customer_code);


--
-- Name: customer customer_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.customer
    ADD CONSTRAINT customer_pkey PRIMARY KEY (cus_id);


--
-- Name: departments departments_dept_code_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.departments
    ADD CONSTRAINT departments_dept_code_key UNIQUE (dept_code);


--
-- Name: departments departments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.departments
    ADD CONSTRAINT departments_pkey PRIMARY KEY (id);


--
-- Name: dept_menu_permissions dept_menu_permissions_dept_code_menu_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dept_menu_permissions
    ADD CONSTRAINT dept_menu_permissions_dept_code_menu_id_key UNIQUE (dept_code, menu_id);


--
-- Name: dept_menu_permissions dept_menu_permissions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dept_menu_permissions
    ADD CONSTRAINT dept_menu_permissions_pkey PRIMARY KEY (id);


--
-- Name: doc_number_counter doc_number_counter_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.doc_number_counter
    ADD CONSTRAINT doc_number_counter_pkey PRIMARY KEY (prefix, year_month);


--
-- Name: erp_audit_log erp_audit_log_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.erp_audit_log
    ADD CONSTRAINT erp_audit_log_pkey PRIMARY KEY (id);


--
-- Name: grn grn_grn_no_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.grn
    ADD CONSTRAINT grn_grn_no_key UNIQUE (grn_no);


--
-- Name: grn_line grn_line_grn_id_line_no_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.grn_line
    ADD CONSTRAINT grn_line_grn_id_line_no_key UNIQUE (grn_id, line_no);


--
-- Name: grn_line grn_line_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.grn_line
    ADD CONSTRAINT grn_line_pkey PRIMARY KEY (id);


--
-- Name: grn grn_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.grn
    ADD CONSTRAINT grn_pkey PRIMARY KEY (id);


--
-- Name: ic_po_receive_document ic_po_receive_document_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_po_receive_document
    ADD CONSTRAINT ic_po_receive_document_pkey PRIMARY KEY (id);


--
-- Name: ic_po_return_document ic_po_return_document_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_po_return_document
    ADD CONSTRAINT ic_po_return_document_pkey PRIMARY KEY (id);


--
-- Name: ic_po_return_document ic_po_return_document_return_no_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_po_return_document
    ADD CONSTRAINT ic_po_return_document_return_no_uq UNIQUE (return_no);


--
-- Name: ic_po_return_line ic_po_return_line_no_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_po_return_line
    ADD CONSTRAINT ic_po_return_line_no_uq UNIQUE (return_id, line_no);


--
-- Name: ic_po_return_line ic_po_return_line_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_po_return_line
    ADD CONSTRAINT ic_po_return_line_pkey PRIMARY KEY (id);


--
-- Name: ic_project_cost_item ic_project_cost_item_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_project_cost_item
    ADD CONSTRAINT ic_project_cost_item_pkey PRIMARY KEY (id);


--
-- Name: ic_project_cost_item ic_project_cost_item_project_code_mat_code_cost_subgroup_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_project_cost_item
    ADD CONSTRAINT ic_project_cost_item_project_code_mat_code_cost_subgroup_id_key UNIQUE (project_code, mat_code, cost_subgroup_id);


--
-- Name: ic_project_cost_item_transaction ic_project_cost_item_transaction_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_project_cost_item_transaction
    ADD CONSTRAINT ic_project_cost_item_transaction_pkey PRIMARY KEY (id);


--
-- Name: ic_project_movement ic_project_movement_doc_no_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_project_movement
    ADD CONSTRAINT ic_project_movement_doc_no_key UNIQUE (doc_no);


--
-- Name: ic_project_movement_line ic_project_movement_line_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_project_movement_line
    ADD CONSTRAINT ic_project_movement_line_pkey PRIMARY KEY (id);


--
-- Name: ic_project_movement ic_project_movement_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_project_movement
    ADD CONSTRAINT ic_project_movement_pkey PRIMARY KEY (id);


--
-- Name: ic_project_receipt_pool ic_project_receipt_pool_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_project_receipt_pool
    ADD CONSTRAINT ic_project_receipt_pool_pkey PRIMARY KEY (id);


--
-- Name: ic_project_receipt_pool_transaction ic_project_receipt_pool_transaction_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_project_receipt_pool_transaction
    ADD CONSTRAINT ic_project_receipt_pool_transaction_pkey PRIMARY KEY (id);


--
-- Name: inventory inventory_mat_code_warehouse_code_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inventory
    ADD CONSTRAINT inventory_mat_code_warehouse_code_key UNIQUE (mat_code, warehouse_code);


--
-- Name: inventory inventory_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inventory
    ADD CONSTRAINT inventory_pkey PRIMARY KEY (id);


--
-- Name: inventory_transaction inventory_transaction_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inventory_transaction
    ADD CONSTRAINT inventory_transaction_pkey PRIMARY KEY (id);


--
-- Name: inventory_transaction inventory_transaction_txn_no_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inventory_transaction
    ADD CONSTRAINT inventory_transaction_txn_no_key UNIQUE (txn_no);


--
-- Name: location location_location_code_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.location
    ADD CONSTRAINT location_location_code_key UNIQUE (location_code);


--
-- Name: location location_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.location
    ADD CONSTRAINT location_pkey PRIMARY KEY (id);


--
-- Name: mat_group mat_group_group_code_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mat_group
    ADD CONSTRAINT mat_group_group_code_key UNIQUE (group_code);


--
-- Name: mat_group mat_group_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mat_group
    ADD CONSTRAINT mat_group_pkey PRIMARY KEY (id);


--
-- Name: mat_name mat_name_mat_name_code_subgroup_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mat_name
    ADD CONSTRAINT mat_name_mat_name_code_subgroup_id_key UNIQUE (mat_name_code, subgroup_id);


--
-- Name: mat_name mat_name_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mat_name
    ADD CONSTRAINT mat_name_pkey PRIMARY KEY (id);


--
-- Name: material_code material_code_mat_code_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.material_code
    ADD CONSTRAINT material_code_mat_code_key UNIQUE (mat_code);


--
-- Name: material_code material_code_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.material_code
    ADD CONSTRAINT material_code_pkey PRIMARY KEY (id);


--
-- Name: memo_attachment memo_attachment_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.memo_attachment
    ADD CONSTRAINT memo_attachment_pkey PRIMARY KEY (id);


--
-- Name: memo_line memo_line_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.memo_line
    ADD CONSTRAINT memo_line_pkey PRIMARY KEY (id);


--
-- Name: memo_line memo_line_unique_line_no; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.memo_line
    ADD CONSTRAINT memo_line_unique_line_no UNIQUE (memo_id, line_no);


--
-- Name: memo memo_memo_no_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.memo
    ADD CONSTRAINT memo_memo_no_key UNIQUE (memo_no);


--
-- Name: memo_number_counter memo_number_counter_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.memo_number_counter
    ADD CONSTRAINT memo_number_counter_pkey PRIMARY KEY (year_month);


--
-- Name: memo memo_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.memo
    ADD CONSTRAINT memo_pkey PRIMARY KEY (id);


--
-- Name: memo_status_log memo_status_log_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.memo_status_log
    ADD CONSTRAINT memo_status_log_pkey PRIMARY KEY (id);


--
-- Name: menus menus_menu_code_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.menus
    ADD CONSTRAINT menus_menu_code_key UNIQUE (menu_code);


--
-- Name: menus menus_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.menus
    ADD CONSTRAINT menus_pkey PRIMARY KEY (id);


--
-- Name: modules modules_module_code_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.modules
    ADD CONSTRAINT modules_module_code_key UNIQUE (module_code);


--
-- Name: modules modules_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.modules
    ADD CONSTRAINT modules_pkey PRIMARY KEY (id);


--
-- Name: payment_installment payment_installment_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payment_installment
    ADD CONSTRAINT payment_installment_pkey PRIMARY KEY (id);


--
-- Name: payment_log payment_log_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payment_log
    ADD CONSTRAINT payment_log_pkey PRIMARY KEY (id);


--
-- Name: pc_number_counter pc_number_counter_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pc_number_counter
    ADD CONSTRAINT pc_number_counter_pkey PRIMARY KEY (year_month);


--
-- Name: permission_audit_logs permission_audit_logs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.permission_audit_logs
    ADD CONSTRAINT permission_audit_logs_pkey PRIMARY KEY (id);


--
-- Name: permissions permissions_permission_key_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.permissions
    ADD CONSTRAINT permissions_permission_key_key UNIQUE (permission_key);


--
-- Name: permissions permissions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.permissions
    ADD CONSTRAINT permissions_pkey PRIMARY KEY (id);


--
-- Name: petty_cash_attachment petty_cash_attachment_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.petty_cash_attachment
    ADD CONSTRAINT petty_cash_attachment_pkey PRIMARY KEY (id);


--
-- Name: petty_cash_requisition_line petty_cash_requisition_line_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.petty_cash_requisition_line
    ADD CONSTRAINT petty_cash_requisition_line_pkey PRIMARY KEY (id);


--
-- Name: petty_cash_requisition petty_cash_requisition_pc_no_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.petty_cash_requisition
    ADD CONSTRAINT petty_cash_requisition_pc_no_key UNIQUE (pc_no);


--
-- Name: petty_cash_requisition petty_cash_requisition_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.petty_cash_requisition
    ADD CONSTRAINT petty_cash_requisition_pkey PRIMARY KEY (id);


--
-- Name: petty_cash_status_log petty_cash_status_log_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.petty_cash_status_log
    ADD CONSTRAINT petty_cash_status_log_pkey PRIMARY KEY (id);


--
-- Name: po_attachment po_attachment_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.po_attachment
    ADD CONSTRAINT po_attachment_pkey PRIMARY KEY (id);


--
-- Name: po_edit_log po_edit_log_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.po_edit_log
    ADD CONSTRAINT po_edit_log_pkey PRIMARY KEY (id);


--
-- Name: po_number_counter po_number_counter_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.po_number_counter
    ADD CONSTRAINT po_number_counter_pkey PRIMARY KEY (year_month);


--
-- Name: po_status_log po_status_log_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.po_status_log
    ADD CONSTRAINT po_status_log_pkey PRIMARY KEY (id);


--
-- Name: por_number_counter por_number_counter_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.por_number_counter
    ADD CONSTRAINT por_number_counter_pkey PRIMARY KEY (year_month);


--
-- Name: pr_attachment pr_attachment_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pr_attachment
    ADD CONSTRAINT pr_attachment_pkey PRIMARY KEY (id);


--
-- Name: pr_number_counter pr_number_counter_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pr_number_counter
    ADD CONSTRAINT pr_number_counter_pkey PRIMARY KEY (year_month);


--
-- Name: pr_status_log pr_status_log_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pr_status_log
    ADD CONSTRAINT pr_status_log_pkey PRIMARY KEY (id);


--
-- Name: project_code_counter project_code_counter_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.project_code_counter
    ADD CONSTRAINT project_code_counter_pkey PRIMARY KEY (code_year, cus_id);


--
-- Name: project_code_year_counter project_code_year_counter_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.project_code_year_counter
    ADD CONSTRAINT project_code_year_counter_pkey PRIMARY KEY (code_year);


--
-- Name: project_cost_budget project_cost_budget_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.project_cost_budget
    ADD CONSTRAINT project_cost_budget_pkey PRIMARY KEY (id);


--
-- Name: project_job_share project_job_share_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.project_job_share
    ADD CONSTRAINT project_job_share_pkey PRIMARY KEY (project_id, job_code);


--
-- Name: project project_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.project
    ADD CONSTRAINT project_pkey PRIMARY KEY (id);


--
-- Name: project project_project_code_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.project
    ADD CONSTRAINT project_project_code_key UNIQUE (project_code);


--
-- Name: project_responsible_person project_responsible_person_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.project_responsible_person
    ADD CONSTRAINT project_responsible_person_pkey PRIMARY KEY (project_id, user_id);


--
-- Name: project_stock project_stock_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.project_stock
    ADD CONSTRAINT project_stock_pkey PRIMARY KEY (id);


--
-- Name: project_stock project_stock_project_code_mat_code_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.project_stock
    ADD CONSTRAINT project_stock_project_code_mat_code_key UNIQUE (project_code, mat_code);


--
-- Name: purchase_order_line purchase_order_line_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_order_line
    ADD CONSTRAINT purchase_order_line_pkey PRIMARY KEY (id);


--
-- Name: purchase_order_line purchase_order_line_po_id_line_no_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_order_line
    ADD CONSTRAINT purchase_order_line_po_id_line_no_key UNIQUE (po_id, line_no);


--
-- Name: purchase_order purchase_order_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_order
    ADD CONSTRAINT purchase_order_pkey PRIMARY KEY (id);


--
-- Name: purchase_order purchase_order_po_no_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_order
    ADD CONSTRAINT purchase_order_po_no_key UNIQUE (po_no);


--
-- Name: purchase_request_line purchase_request_line_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_request_line
    ADD CONSTRAINT purchase_request_line_pkey PRIMARY KEY (id);


--
-- Name: purchase_request_line purchase_request_line_pr_id_line_no_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_request_line
    ADD CONSTRAINT purchase_request_line_pr_id_line_no_key UNIQUE (pr_id, line_no);


--
-- Name: purchase_request purchase_request_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_request
    ADD CONSTRAINT purchase_request_pkey PRIMARY KEY (id);


--
-- Name: purchase_request purchase_request_pr_no_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_request
    ADD CONSTRAINT purchase_request_pr_no_key UNIQUE (pr_no);


--
-- Name: requisition_line requisition_line_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.requisition_line
    ADD CONSTRAINT requisition_line_pkey PRIMARY KEY (id);


--
-- Name: requisition requisition_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.requisition
    ADD CONSTRAINT requisition_pkey PRIMARY KEY (id);


--
-- Name: requisition requisition_req_no_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.requisition
    ADD CONSTRAINT requisition_req_no_key UNIQUE (req_no);


--
-- Name: requisition_status_log requisition_status_log_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.requisition_status_log
    ADD CONSTRAINT requisition_status_log_pkey PRIMARY KEY (id);


--
-- Name: ret_number_counter ret_number_counter_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ret_number_counter
    ADD CONSTRAINT ret_number_counter_pkey PRIMARY KEY (year_month);


--
-- Name: rfq_line rfq_line_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rfq_line
    ADD CONSTRAINT rfq_line_pkey PRIMARY KEY (id);


--
-- Name: rfq_line rfq_line_rfq_id_line_no_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rfq_line
    ADD CONSTRAINT rfq_line_rfq_id_line_no_key UNIQUE (rfq_id, line_no);


--
-- Name: rfq rfq_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rfq
    ADD CONSTRAINT rfq_pkey PRIMARY KEY (id);


--
-- Name: rfq rfq_rfq_no_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rfq
    ADD CONSTRAINT rfq_rfq_no_key UNIQUE (rfq_no);


--
-- Name: role_menu_permissions role_menu_permissions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.role_menu_permissions
    ADD CONSTRAINT role_menu_permissions_pkey PRIMARY KEY (id);


--
-- Name: role_menu_permissions role_menu_permissions_role_id_menu_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.role_menu_permissions
    ADD CONSTRAINT role_menu_permissions_role_id_menu_id_key UNIQUE (role_id, menu_id);


--
-- Name: role_menus role_menus_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.role_menus
    ADD CONSTRAINT role_menus_pkey PRIMARY KEY (role_id, menu_id);


--
-- Name: role_permissions role_permissions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.role_permissions
    ADD CONSTRAINT role_permissions_pkey PRIMARY KEY (role_id, permission_id);


--
-- Name: roles roles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.roles
    ADD CONSTRAINT roles_pkey PRIMARY KEY (id);


--
-- Name: roles roles_role_code_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.roles
    ADD CONSTRAINT roles_role_code_key UNIQUE (role_code);


--
-- Name: schema_migrations schema_migrations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.schema_migrations
    ADD CONSTRAINT schema_migrations_pkey PRIMARY KEY (version);


--
-- Name: spec_size spec_size_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.spec_size
    ADD CONSTRAINT spec_size_pkey PRIMARY KEY (id);


--
-- Name: spec_size spec_size_spec_code_mat_name_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.spec_size
    ADD CONSTRAINT spec_size_spec_code_mat_name_id_key UNIQUE (spec_code, mat_name_id);


--
-- Name: stock_category stock_category_code_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_category
    ADD CONSTRAINT stock_category_code_uq UNIQUE (code);


--
-- Name: stock_category stock_category_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_category
    ADD CONSTRAINT stock_category_pkey PRIMARY KEY (id);


--
-- Name: stock_count stock_count_count_no_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_count
    ADD CONSTRAINT stock_count_count_no_key UNIQUE (count_no);


--
-- Name: stock_count_line stock_count_line_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_count_line
    ADD CONSTRAINT stock_count_line_pkey PRIMARY KEY (id);


--
-- Name: stock_count stock_count_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_count
    ADD CONSTRAINT stock_count_pkey PRIMARY KEY (id);


--
-- Name: stock_inventory stock_inventory_item_loc_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_inventory
    ADD CONSTRAINT stock_inventory_item_loc_uq UNIQUE (item_id, location_code);


--
-- Name: stock_inventory stock_inventory_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_inventory
    ADD CONSTRAINT stock_inventory_pkey PRIMARY KEY (id);


--
-- Name: stock_item stock_item_code_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_item
    ADD CONSTRAINT stock_item_code_uq UNIQUE (mat_code);


--
-- Name: stock_item_image stock_item_image_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_item_image
    ADD CONSTRAINT stock_item_image_pkey PRIMARY KEY (id);


--
-- Name: stock_item stock_item_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_item
    ADD CONSTRAINT stock_item_pkey PRIMARY KEY (id);


--
-- Name: stock_reservation stock_res_no_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_reservation
    ADD CONSTRAINT stock_res_no_uq UNIQUE (reservation_no);


--
-- Name: stock_reservation stock_res_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_reservation
    ADD CONSTRAINT stock_res_pkey PRIMARY KEY (id);


--
-- Name: stock_transfer_line stock_transfer_line_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_transfer_line
    ADD CONSTRAINT stock_transfer_line_pkey PRIMARY KEY (id);


--
-- Name: stock_transfer stock_transfer_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_transfer
    ADD CONSTRAINT stock_transfer_pkey PRIMARY KEY (id);


--
-- Name: stock_transfer_status_log stock_transfer_status_log_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_transfer_status_log
    ADD CONSTRAINT stock_transfer_status_log_pkey PRIMARY KEY (id);


--
-- Name: stock_transfer stock_transfer_transfer_no_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_transfer
    ADD CONSTRAINT stock_transfer_transfer_no_key UNIQUE (transfer_no);


--
-- Name: stock_transaction stock_txn_no_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_transaction
    ADD CONSTRAINT stock_txn_no_uq UNIQUE (txn_no);


--
-- Name: stock_transaction stock_txn_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_transaction
    ADD CONSTRAINT stock_txn_pkey PRIMARY KEY (id);


--
-- Name: storage_zone storage_zone_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.storage_zone
    ADD CONSTRAINT storage_zone_pkey PRIMARY KEY (id);


--
-- Name: storage_zone storage_zone_warehouse_id_zone_code_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.storage_zone
    ADD CONSTRAINT storage_zone_warehouse_id_zone_code_key UNIQUE (warehouse_id, zone_code);


--
-- Name: subgroup subgroup_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subgroup
    ADD CONSTRAINT subgroup_pkey PRIMARY KEY (id);


--
-- Name: subgroup subgroup_subgroup_code_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subgroup
    ADD CONSTRAINT subgroup_subgroup_code_key UNIQUE (subgroup_code);


--
-- Name: supplier supplier_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.supplier
    ADD CONSTRAINT supplier_pkey PRIMARY KEY (id);


--
-- Name: unit unit_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.unit
    ADD CONSTRAINT unit_pkey PRIMARY KEY (id);


--
-- Name: unit unit_unit_code_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.unit
    ADD CONSTRAINT unit_unit_code_key UNIQUE (unit_code);


--
-- Name: ic_project_receipt_pool uq_ic_project_receipt_pool; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_project_receipt_pool
    ADD CONSTRAINT uq_ic_project_receipt_pool UNIQUE (project_code, mat_code, cost_subgroup_id);


--
-- Name: payment_installment uq_payment_installment_doc_no; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payment_installment
    ADD CONSTRAINT uq_payment_installment_doc_no UNIQUE (doc_type, doc_id, installment_no);


--
-- Name: project_cost_budget uq_project_cost_budget_project_subgroup; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.project_cost_budget
    ADD CONSTRAINT uq_project_cost_budget_project_subgroup UNIQUE (project_id, cost_subgroup_id);


--
-- Name: work_order_cost_code uq_wo_cost_code_wo_id_code; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.work_order_cost_code
    ADD CONSTRAINT uq_wo_cost_code_wo_id_code UNIQUE (wo_id, cost_code);


--
-- Name: user_menu_permissions user_menu_permissions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_menu_permissions
    ADD CONSTRAINT user_menu_permissions_pkey PRIMARY KEY (id);


--
-- Name: user_menu_permissions user_menu_permissions_user_id_menu_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_menu_permissions
    ADD CONSTRAINT user_menu_permissions_user_id_menu_id_key UNIQUE (user_id, menu_id);


--
-- Name: user_permissions user_permissions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_permissions
    ADD CONSTRAINT user_permissions_pkey PRIMARY KEY (user_id, permission_id);


--
-- Name: user_roles user_roles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_roles
    ADD CONSTRAINT user_roles_pkey PRIMARY KEY (user_id, role_id);


--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);


--
-- Name: users users_username_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_username_key UNIQUE (username);


--
-- Name: warehouse warehouse_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.warehouse
    ADD CONSTRAINT warehouse_pkey PRIMARY KEY (id);


--
-- Name: warehouse warehouse_warehouse_code_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.warehouse
    ADD CONSTRAINT warehouse_warehouse_code_key UNIQUE (warehouse_code);


--
-- Name: wht_income_type wht_income_type_code_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wht_income_type
    ADD CONSTRAINT wht_income_type_code_key UNIQUE (code);


--
-- Name: wht_income_type wht_income_type_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wht_income_type
    ADD CONSTRAINT wht_income_type_pkey PRIMARY KEY (id);


--
-- Name: wo_contractor_log_bill_invoice wo_contractor_log_bill_invoice_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wo_contractor_log_bill_invoice
    ADD CONSTRAINT wo_contractor_log_bill_invoice_pkey PRIMARY KEY (id);


--
-- Name: wo_contractor_log_bill_line wo_contractor_log_bill_line_bill_id_line_code_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wo_contractor_log_bill_line
    ADD CONSTRAINT wo_contractor_log_bill_line_bill_id_line_code_key UNIQUE (bill_id, line_code);


--
-- Name: wo_contractor_log_bill_line wo_contractor_log_bill_line_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wo_contractor_log_bill_line
    ADD CONSTRAINT wo_contractor_log_bill_line_pkey PRIMARY KEY (id);


--
-- Name: wo_contractor_log_bill wo_contractor_log_bill_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wo_contractor_log_bill
    ADD CONSTRAINT wo_contractor_log_bill_pkey PRIMARY KEY (id);


--
-- Name: wo_contractor_log wo_contractor_log_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wo_contractor_log
    ADD CONSTRAINT wo_contractor_log_pkey PRIMARY KEY (id);


--
-- Name: wo_contractor_log_term wo_contractor_log_term_log_id_item_type_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wo_contractor_log_term
    ADD CONSTRAINT wo_contractor_log_term_log_id_item_type_key UNIQUE (log_id, item_type);


--
-- Name: wo_contractor_log_term wo_contractor_log_term_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wo_contractor_log_term
    ADD CONSTRAINT wo_contractor_log_term_pkey PRIMARY KEY (id);


--
-- Name: work_order_attachment work_order_attachment_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.work_order_attachment
    ADD CONSTRAINT work_order_attachment_pkey PRIMARY KEY (id);


--
-- Name: work_order_cost_code work_order_cost_code_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.work_order_cost_code
    ADD CONSTRAINT work_order_cost_code_pkey PRIMARY KEY (id);


--
-- Name: work_order_edit_log work_order_edit_log_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.work_order_edit_log
    ADD CONSTRAINT work_order_edit_log_pkey PRIMARY KEY (id);


--
-- Name: work_order_line work_order_line_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.work_order_line
    ADD CONSTRAINT work_order_line_pkey PRIMARY KEY (id);


--
-- Name: work_order_payment_installment work_order_payment_installment_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.work_order_payment_installment
    ADD CONSTRAINT work_order_payment_installment_pkey PRIMARY KEY (id);


--
-- Name: work_order_penalty work_order_penalty_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.work_order_penalty
    ADD CONSTRAINT work_order_penalty_pkey PRIMARY KEY (id);


--
-- Name: work_order work_order_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.work_order
    ADD CONSTRAINT work_order_pkey PRIMARY KEY (id);


--
-- Name: work_order_retention work_order_retention_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.work_order_retention
    ADD CONSTRAINT work_order_retention_pkey PRIMARY KEY (id);


--
-- Name: work_order_status_log work_order_status_log_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.work_order_status_log
    ADD CONSTRAINT work_order_status_log_pkey PRIMARY KEY (id);


--
-- Name: work_order work_order_wo_no_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.work_order
    ADD CONSTRAINT work_order_wo_no_key UNIQUE (wo_no);


--
-- Name: idx_approval_log_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_approval_log_at ON public.approval_log USING btree (action_at);


--
-- Name: idx_approval_log_by; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_approval_log_by ON public.approval_log USING btree (action_by);


--
-- Name: idx_approval_log_doc; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_approval_log_doc ON public.approval_log USING btree (doc_type, doc_id);


--
-- Name: idx_approval_req_assignee; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_approval_req_assignee ON public.approval_request USING btree (assigned_to);


--
-- Name: idx_approval_req_doc; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_approval_req_doc ON public.approval_request USING btree (doc_type, doc_id);


--
-- Name: idx_approval_req_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_approval_req_status ON public.approval_request USING btree (status);


--
-- Name: idx_borrow_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_borrow_status ON public.borrow USING btree (status);


--
-- Name: idx_borrow_status_log_borrow; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_borrow_status_log_borrow ON public.borrow_status_log USING btree (borrow_id);


--
-- Name: idx_cost_group_job_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cost_group_job_id ON public.cost_group USING btree (job_id);


--
-- Name: idx_cost_item_txn_return_document_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cost_item_txn_return_document_id ON public.ic_project_cost_item_transaction USING btree (return_document_id);


--
-- Name: idx_cost_job_subject_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cost_job_subject_id ON public.cost_job USING btree (subject_id);


--
-- Name: idx_cost_subgroup_group_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cost_subgroup_group_id ON public.cost_subgroup USING btree (group_id);


--
-- Name: idx_customer_is_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_customer_is_active ON public.customer USING btree (is_active);


--
-- Name: idx_customer_name; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_customer_name ON public.customer USING btree (customer_name);


--
-- Name: idx_erp_audit_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_erp_audit_at ON public.erp_audit_log USING btree (changed_at);


--
-- Name: idx_erp_audit_table; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_erp_audit_table ON public.erp_audit_log USING btree (table_name, record_id);


--
-- Name: idx_grn_date; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_grn_date ON public.grn USING btree (grn_date);


--
-- Name: idx_grn_po; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_grn_po ON public.grn USING btree (po_id);


--
-- Name: idx_ic_pcit_receive_document_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ic_pcit_receive_document_id ON public.ic_project_cost_item_transaction USING btree (receive_document_id);


--
-- Name: idx_ic_po_receive_document_po_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ic_po_receive_document_po_id ON public.ic_po_receive_document USING btree (po_id);


--
-- Name: idx_ic_po_return_document_po_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ic_po_return_document_po_id ON public.ic_po_return_document USING btree (po_id);


--
-- Name: idx_ic_po_return_line_return_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ic_po_return_line_return_id ON public.ic_po_return_line USING btree (return_id);


--
-- Name: idx_ic_project_cost_item_project_code; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ic_project_cost_item_project_code ON public.ic_project_cost_item USING btree (project_code);


--
-- Name: idx_ic_project_cost_item_txn_movement; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ic_project_cost_item_txn_movement ON public.ic_project_cost_item_transaction USING btree (movement_id);


--
-- Name: idx_ic_project_cost_item_txn_po_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ic_project_cost_item_txn_po_id ON public.ic_project_cost_item_transaction USING btree (po_id);


--
-- Name: idx_ic_project_movement_line_movement; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ic_project_movement_line_movement ON public.ic_project_movement_line USING btree (movement_id);


--
-- Name: idx_ic_project_movement_project; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ic_project_movement_project ON public.ic_project_movement USING btree (project_code);


--
-- Name: idx_ic_project_receipt_pool_project; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ic_project_receipt_pool_project ON public.ic_project_receipt_pool USING btree (project_code);


--
-- Name: idx_ic_project_receipt_pool_txn_pool; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ic_project_receipt_pool_txn_pool ON public.ic_project_receipt_pool_transaction USING btree (receipt_pool_id);


--
-- Name: idx_inv_txn_date; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_inv_txn_date ON public.inventory_transaction USING btree (txn_date);


--
-- Name: idx_inv_txn_mat; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_inv_txn_mat ON public.inventory_transaction USING btree (mat_code);


--
-- Name: idx_inv_txn_ref; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_inv_txn_ref ON public.inventory_transaction USING btree (ref_doc_type, ref_doc_no);


--
-- Name: idx_inv_txn_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_inv_txn_type ON public.inventory_transaction USING btree (txn_type);


--
-- Name: idx_inventory_mat; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_inventory_mat ON public.inventory USING btree (mat_code);


--
-- Name: idx_inventory_wh; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_inventory_wh ON public.inventory USING btree (warehouse_code);


--
-- Name: idx_material_code_brand_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_material_code_brand_id ON public.material_code USING btree (brand_id);


--
-- Name: idx_material_code_group_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_material_code_group_id ON public.material_code USING btree (group_id);


--
-- Name: idx_material_code_mat_code; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_material_code_mat_code ON public.material_code USING btree (mat_code);


--
-- Name: idx_material_code_mat_name_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_material_code_mat_name_id ON public.material_code USING btree (mat_name_id);


--
-- Name: idx_material_code_spec_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_material_code_spec_id ON public.material_code USING btree (spec_id);


--
-- Name: idx_material_code_subgroup_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_material_code_subgroup_id ON public.material_code USING btree (subgroup_id);


--
-- Name: idx_material_code_unit_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_material_code_unit_id ON public.material_code USING btree (unit_id);


--
-- Name: idx_material_cost_subgroup; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_material_cost_subgroup ON public.material_code USING btree (cost_subgroup_id);


--
-- Name: idx_memo_approver_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_memo_approver_id ON public.memo USING btree (approver_id);


--
-- Name: idx_memo_attachment_memo_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_memo_attachment_memo_id ON public.memo_attachment USING btree (memo_id);


--
-- Name: idx_memo_line_memo_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_memo_line_memo_id ON public.memo_line USING btree (memo_id);


--
-- Name: idx_memo_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_memo_status ON public.memo USING btree (status);


--
-- Name: idx_memo_status_log_memo_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_memo_status_log_memo_id ON public.memo_status_log USING btree (memo_id);


--
-- Name: idx_payment_installment_doc; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payment_installment_doc ON public.payment_installment USING btree (doc_type, doc_id);


--
-- Name: idx_payment_installment_payment_log; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payment_installment_payment_log ON public.payment_installment USING btree (payment_log_id);


--
-- Name: idx_payment_log_doc; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payment_log_doc ON public.payment_log USING btree (doc_type, doc_id);


--
-- Name: idx_pc_requested_by; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_pc_requested_by ON public.petty_cash_requisition USING btree (requested_by);


--
-- Name: idx_pc_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_pc_status ON public.petty_cash_requisition USING btree (status);


--
-- Name: idx_pc_status_log_pc_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_pc_status_log_pc_id ON public.petty_cash_status_log USING btree (pc_id);


--
-- Name: idx_pcl_mat_code; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_pcl_mat_code ON public.petty_cash_requisition_line USING btree (mat_code);


--
-- Name: idx_pcl_pc_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_pcl_pc_id ON public.petty_cash_requisition_line USING btree (pc_id);


--
-- Name: idx_pcl_project_code; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_pcl_project_code ON public.petty_cash_requisition_line USING btree (project_code);


--
-- Name: idx_pcl_supplier_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_pcl_supplier_id ON public.petty_cash_requisition_line USING btree (supplier_id);


--
-- Name: idx_pcr_pc_date; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_pcr_pc_date ON public.petty_cash_requisition USING btree (pc_date);


--
-- Name: idx_pcr_project_code; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_pcr_project_code ON public.petty_cash_requisition USING btree (project_code);


--
-- Name: idx_pcr_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_pcr_status ON public.petty_cash_requisition USING btree (status);


--
-- Name: idx_petty_cash_attachment_pc_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_petty_cash_attachment_pc_id ON public.petty_cash_attachment USING btree (pc_id);


--
-- Name: idx_po_date; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_po_date ON public.purchase_order USING btree (po_date);


--
-- Name: idx_po_line_pr_line_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_po_line_pr_line_id ON public.purchase_order_line USING btree (pr_line_id);


--
-- Name: idx_po_pr_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_po_pr_id ON public.purchase_order USING btree (pr_id);


--
-- Name: idx_po_project_code; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_po_project_code ON public.purchase_order USING btree (project_code) WHERE (deleted_at IS NULL);


--
-- Name: idx_po_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_po_status ON public.purchase_order USING btree (status);


--
-- Name: idx_po_use_vat; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_po_use_vat ON public.purchase_order USING btree (use_vat);


--
-- Name: idx_po_use_wht; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_po_use_wht ON public.purchase_order USING btree (use_wht);


--
-- Name: idx_po_warehouse_code; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_po_warehouse_code ON public.purchase_order USING btree (warehouse_code);


--
-- Name: idx_pr_date; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_pr_date ON public.purchase_request USING btree (pr_date);


--
-- Name: idx_pr_memo_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_pr_memo_id ON public.purchase_request USING btree (memo_id);


--
-- Name: idx_pr_requested_by; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_pr_requested_by ON public.purchase_request USING btree (requested_by);


--
-- Name: idx_pr_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_pr_status ON public.purchase_request USING btree (status);


--
-- Name: idx_prl_memo_line_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_prl_memo_line_id ON public.purchase_request_line USING btree (memo_line_id);


--
-- Name: idx_project_cost_budget_project_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_project_cost_budget_project_id ON public.project_cost_budget USING btree (project_id);


--
-- Name: idx_project_owner_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_project_owner_id ON public.project USING btree (owner_id);


--
-- Name: idx_project_stock_lookup; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_project_stock_lookup ON public.project_stock USING btree (project_code, mat_code);


--
-- Name: idx_prp_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_prp_user_id ON public.project_responsible_person USING btree (user_id);


--
-- Name: idx_requisition_line_mat; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_requisition_line_mat ON public.requisition_line USING btree (mat_code);


--
-- Name: idx_requisition_line_req; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_requisition_line_req ON public.requisition_line USING btree (req_id);


--
-- Name: idx_requisition_project; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_requisition_project ON public.requisition USING btree (project_code);


--
-- Name: idx_requisition_requester; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_requisition_requester ON public.requisition USING btree (requester_id);


--
-- Name: idx_requisition_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_requisition_status ON public.requisition USING btree (status);


--
-- Name: idx_roles_dept_code; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_roles_dept_code ON public.roles USING btree (dept_code);


--
-- Name: idx_stock_inv_item; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_stock_inv_item ON public.stock_inventory USING btree (item_id);


--
-- Name: idx_stock_inv_location; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_stock_inv_location ON public.stock_inventory USING btree (location_code);


--
-- Name: idx_stock_inv_warehouse; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_stock_inv_warehouse ON public.stock_inventory USING btree (warehouse_code);


--
-- Name: idx_stock_item_category; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_stock_item_category ON public.stock_item USING btree (category_id);


--
-- Name: idx_stock_item_image_item_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_stock_item_image_item_id ON public.stock_item_image USING btree (item_id);


--
-- Name: idx_stock_item_is_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_stock_item_is_active ON public.stock_item USING btree (is_active);


--
-- Name: idx_stock_item_item_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_stock_item_item_type ON public.stock_item USING btree (item_type);


--
-- Name: idx_stock_res_item; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_stock_res_item ON public.stock_reservation USING btree (item_id);


--
-- Name: idx_stock_res_needed; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_stock_res_needed ON public.stock_reservation USING btree (needed_by);


--
-- Name: idx_stock_res_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_stock_res_status ON public.stock_reservation USING btree (status);


--
-- Name: idx_stock_res_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_stock_res_user ON public.stock_reservation USING btree (requested_by);


--
-- Name: idx_stock_transaction_receive_document_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_stock_transaction_receive_document_id ON public.stock_transaction USING btree (receive_document_id);


--
-- Name: idx_stock_transaction_return_document_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_stock_transaction_return_document_id ON public.stock_transaction USING btree (return_document_id);


--
-- Name: idx_stock_transfer_line_transfer; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_stock_transfer_line_transfer ON public.stock_transfer_line USING btree (transfer_id);


--
-- Name: idx_stock_txn_date; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_stock_txn_date ON public.stock_transaction USING btree (txn_date DESC);


--
-- Name: idx_stock_txn_item; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_stock_txn_item ON public.stock_transaction USING btree (item_id);


--
-- Name: idx_stock_txn_ref; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_stock_txn_ref ON public.stock_transaction USING btree (ref_doc_type, ref_doc_id);


--
-- Name: idx_stock_txn_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_stock_txn_type ON public.stock_transaction USING btree (txn_type);


--
-- Name: idx_users_dept_code; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_dept_code ON public.users USING btree (dept_code);


--
-- Name: idx_wo_contractor_log_bill_invoice_bill; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_wo_contractor_log_bill_invoice_bill ON public.wo_contractor_log_bill_invoice USING btree (bill_id);


--
-- Name: idx_wo_contractor_log_bill_log; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_wo_contractor_log_bill_log ON public.wo_contractor_log_bill USING btree (log_id);


--
-- Name: idx_wo_contractor_log_wo; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_wo_contractor_log_wo ON public.wo_contractor_log USING btree (wo_id);


--
-- Name: idx_wo_cost_code_wo_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_wo_cost_code_wo_id ON public.work_order_cost_code USING btree (wo_id);


--
-- Name: idx_wo_payment_installment_wo_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_wo_payment_installment_wo_id ON public.work_order_payment_installment USING btree (wo_id);


--
-- Name: idx_wo_penalty_wo_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_wo_penalty_wo_id ON public.work_order_penalty USING btree (wo_id);


--
-- Name: idx_wo_retention_wo_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_wo_retention_wo_id ON public.work_order_retention USING btree (wo_id);


--
-- Name: idx_work_order_edit_log_wo; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_work_order_edit_log_wo ON public.work_order_edit_log USING btree (wo_id);


--
-- Name: idx_work_order_line_wo_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_work_order_line_wo_id ON public.work_order_line USING btree (wo_id);


--
-- Name: idx_work_order_project; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_work_order_project ON public.work_order USING btree (project_code);


--
-- Name: idx_work_order_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_work_order_status ON public.work_order USING btree (status);


--
-- Name: payment_log_reverses_uq; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX payment_log_reverses_uq ON public.payment_log USING btree (reverses_id) WHERE (reverses_id IS NOT NULL);


--
-- Name: project_customer_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX project_customer_id_idx ON public.project USING btree (customer_id);


--
-- Name: uq_ic_po_receive_document_receive_no; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_ic_po_receive_document_receive_no ON public.ic_po_receive_document USING btree (receive_no) WHERE (receive_no IS NOT NULL);


--
-- Name: role_menu_permissions trg_enforce_read; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER trg_enforce_read BEFORE INSERT OR UPDATE ON public.role_menu_permissions FOR EACH ROW EXECUTE FUNCTION public.enforce_read_before_write();


--
-- Name: project_cost_budget trg_project_cost_budget_item_only; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER trg_project_cost_budget_item_only BEFORE INSERT OR UPDATE OF cost_subgroup_id ON public.project_cost_budget FOR EACH ROW EXECUTE FUNCTION public.trg_project_cost_budget_item_only();


--
-- Name: project_job_share trg_project_job_share_total; Type: TRIGGER; Schema: public; Owner: -
--

CREATE CONSTRAINT TRIGGER trg_project_job_share_total AFTER INSERT OR UPDATE ON public.project_job_share DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION public.chk_project_job_share_total();


--
-- Name: approval_config approval_config_approver_role_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_config
    ADD CONSTRAINT approval_config_approver_role_id_fkey FOREIGN KEY (approver_role_id) REFERENCES public.roles(id);


--
-- Name: approval_config approval_config_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_config
    ADD CONSTRAINT approval_config_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: approval_config approval_config_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_config
    ADD CONSTRAINT approval_config_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- Name: approval_delegation approval_delegation_created_by_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_delegation
    ADD CONSTRAINT approval_delegation_created_by_fk FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: approval_delegation approval_delegation_user_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_delegation
    ADD CONSTRAINT approval_delegation_user_fk FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: approval_doc_types approval_doc_types_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_doc_types
    ADD CONSTRAINT approval_doc_types_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: approval_doc_types approval_doc_types_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_doc_types
    ADD CONSTRAINT approval_doc_types_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- Name: approval_log approval_log_action_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_log
    ADD CONSTRAINT approval_log_action_by_fkey FOREIGN KEY (action_by) REFERENCES public.users(id);


--
-- Name: approval_log approval_log_approval_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_log
    ADD CONSTRAINT approval_log_approval_id_fkey FOREIGN KEY (approval_id) REFERENCES public.approval_request(id);


--
-- Name: approval_request approval_request_assigned_to_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_request
    ADD CONSTRAINT approval_request_assigned_to_fkey FOREIGN KEY (assigned_to) REFERENCES public.users(id);


--
-- Name: approval_request approval_request_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_request
    ADD CONSTRAINT approval_request_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: approval_request approval_request_requested_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_request
    ADD CONSTRAINT approval_request_requested_by_fkey FOREIGN KEY (requested_by) REFERENCES public.users(id);


--
-- Name: approval_request approval_request_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_request
    ADD CONSTRAINT approval_request_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- Name: borrow borrow_approved_by_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.borrow
    ADD CONSTRAINT borrow_approved_by_fk FOREIGN KEY (approved_by) REFERENCES public.users(id);


--
-- Name: borrow borrow_borrower_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.borrow
    ADD CONSTRAINT borrow_borrower_id_fkey FOREIGN KEY (borrower_id) REFERENCES public.users(id);


--
-- Name: borrow borrow_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.borrow
    ADD CONSTRAINT borrow_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: borrow_line borrow_line_borrow_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.borrow_line
    ADD CONSTRAINT borrow_line_borrow_id_fkey FOREIGN KEY (borrow_id) REFERENCES public.borrow(id) ON DELETE CASCADE;


--
-- Name: borrow_line borrow_line_mat_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.borrow_line
    ADD CONSTRAINT borrow_line_mat_code_fkey FOREIGN KEY (mat_code) REFERENCES public.material_code(mat_code) DEFERRABLE;


--
-- Name: borrow_line borrow_line_stock_item_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.borrow_line
    ADD CONSTRAINT borrow_line_stock_item_fk FOREIGN KEY (stock_item_id) REFERENCES public.stock_item(id) ON DELETE RESTRICT;


--
-- Name: borrow borrow_location_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.borrow
    ADD CONSTRAINT borrow_location_code_fkey FOREIGN KEY (location_code) REFERENCES public.location(location_code);


--
-- Name: borrow_status_log borrow_status_log_borrow_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.borrow_status_log
    ADD CONSTRAINT borrow_status_log_borrow_fk FOREIGN KEY (borrow_id) REFERENCES public.borrow(id) ON DELETE CASCADE;


--
-- Name: borrow_status_log borrow_status_log_user_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.borrow_status_log
    ADD CONSTRAINT borrow_status_log_user_fk FOREIGN KEY (changed_by) REFERENCES public.users(id);


--
-- Name: borrow borrow_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.borrow
    ADD CONSTRAINT borrow_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- Name: borrow borrow_warehouse_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.borrow
    ADD CONSTRAINT borrow_warehouse_code_fkey FOREIGN KEY (warehouse_code) REFERENCES public.warehouse(warehouse_code);


--
-- Name: brand brand_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.brand
    ADD CONSTRAINT brand_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: brand brand_spec_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.brand
    ADD CONSTRAINT brand_spec_id_fkey FOREIGN KEY (spec_id) REFERENCES public.spec_size(id);


--
-- Name: brand brand_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.brand
    ADD CONSTRAINT brand_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- Name: cost_group cost_group_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cost_group
    ADD CONSTRAINT cost_group_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: cost_group cost_group_job_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cost_group
    ADD CONSTRAINT cost_group_job_id_fkey FOREIGN KEY (job_id) REFERENCES public.cost_job(id) NOT VALID;


--
-- Name: cost_group cost_group_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cost_group
    ADD CONSTRAINT cost_group_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- Name: cost_job cost_job_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cost_job
    ADD CONSTRAINT cost_job_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: cost_job cost_job_subject_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cost_job
    ADD CONSTRAINT cost_job_subject_id_fkey FOREIGN KEY (subject_id) REFERENCES public.cost_subject(id) NOT VALID;


--
-- Name: cost_job cost_job_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cost_job
    ADD CONSTRAINT cost_job_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- Name: cost_subgroup cost_subgroup_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cost_subgroup
    ADD CONSTRAINT cost_subgroup_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: cost_subgroup cost_subgroup_group_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cost_subgroup
    ADD CONSTRAINT cost_subgroup_group_id_fkey FOREIGN KEY (group_id) REFERENCES public.cost_group(id) NOT VALID;


--
-- Name: cost_subgroup cost_subgroup_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cost_subgroup
    ADD CONSTRAINT cost_subgroup_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- Name: cost_subject cost_subject_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cost_subject
    ADD CONSTRAINT cost_subject_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: cost_subject cost_subject_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cost_subject
    ADD CONSTRAINT cost_subject_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- Name: customer customer_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.customer
    ADD CONSTRAINT customer_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- Name: dept_menu_permissions dept_menu_permissions_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dept_menu_permissions
    ADD CONSTRAINT dept_menu_permissions_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: dept_menu_permissions dept_menu_permissions_dept_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dept_menu_permissions
    ADD CONSTRAINT dept_menu_permissions_dept_code_fkey FOREIGN KEY (dept_code) REFERENCES public.departments(dept_code) ON UPDATE CASCADE;


--
-- Name: dept_menu_permissions dept_menu_permissions_menu_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dept_menu_permissions
    ADD CONSTRAINT dept_menu_permissions_menu_id_fkey FOREIGN KEY (menu_id) REFERENCES public.menus(id) ON DELETE CASCADE;


--
-- Name: dept_menu_permissions dept_menu_permissions_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dept_menu_permissions
    ADD CONSTRAINT dept_menu_permissions_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- Name: erp_audit_log erp_audit_log_changed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.erp_audit_log
    ADD CONSTRAINT erp_audit_log_changed_by_fkey FOREIGN KEY (changed_by) REFERENCES public.users(id);


--
-- Name: grn fk_grn_supplier_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.grn
    ADD CONSTRAINT fk_grn_supplier_id FOREIGN KEY (supplier_id) REFERENCES public.supplier(id);


--
-- Name: petty_cash_requisition_line fk_pcl_project_code; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.petty_cash_requisition_line
    ADD CONSTRAINT fk_pcl_project_code FOREIGN KEY (project_code) REFERENCES public.project(project_code);


--
-- Name: petty_cash_requisition_line fk_pcl_supplier; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.petty_cash_requisition_line
    ADD CONSTRAINT fk_pcl_supplier FOREIGN KEY (supplier_id) REFERENCES public.supplier(id);


--
-- Name: petty_cash_requisition_line fk_pcl_wht_income_type; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.petty_cash_requisition_line
    ADD CONSTRAINT fk_pcl_wht_income_type FOREIGN KEY (wht_income_type_id) REFERENCES public.wht_income_type(id);


--
-- Name: purchase_order fk_po_supplier_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_order
    ADD CONSTRAINT fk_po_supplier_id FOREIGN KEY (supplier_id) REFERENCES public.supplier(id);


--
-- Name: pr_attachment fk_pr_attachment_pr; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pr_attachment
    ADD CONSTRAINT fk_pr_attachment_pr FOREIGN KEY (pr_id) REFERENCES public.purchase_request(id);


--
-- Name: rfq fk_rfq_supplier_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rfq
    ADD CONSTRAINT fk_rfq_supplier_id FOREIGN KEY (supplier_id) REFERENCES public.supplier(id);


--
-- Name: roles fk_roles_dept_code; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.roles
    ADD CONSTRAINT fk_roles_dept_code FOREIGN KEY (dept_code) REFERENCES public.departments(dept_code) ON UPDATE CASCADE;


--
-- Name: users fk_users_dept_code; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT fk_users_dept_code FOREIGN KEY (dept_code) REFERENCES public.departments(dept_code) ON UPDATE CASCADE;


--
-- Name: work_order fk_wo_supplier_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.work_order
    ADD CONSTRAINT fk_wo_supplier_id FOREIGN KEY (supplier_id) REFERENCES public.supplier(id);


--
-- Name: grn grn_confirmed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.grn
    ADD CONSTRAINT grn_confirmed_by_fkey FOREIGN KEY (confirmed_by) REFERENCES public.users(id);


--
-- Name: grn grn_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.grn
    ADD CONSTRAINT grn_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: grn_line grn_line_grn_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.grn_line
    ADD CONSTRAINT grn_line_grn_id_fkey FOREIGN KEY (grn_id) REFERENCES public.grn(id) ON DELETE CASCADE;


--
-- Name: grn_line grn_line_mat_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.grn_line
    ADD CONSTRAINT grn_line_mat_code_fkey FOREIGN KEY (mat_code) REFERENCES public.material_code(mat_code) DEFERRABLE;


--
-- Name: grn_line grn_line_po_line_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.grn_line
    ADD CONSTRAINT grn_line_po_line_id_fkey FOREIGN KEY (po_line_id) REFERENCES public.purchase_order_line(id);


--
-- Name: grn_line grn_line_zone_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.grn_line
    ADD CONSTRAINT grn_line_zone_id_fkey FOREIGN KEY (zone_id) REFERENCES public.storage_zone(id);


--
-- Name: grn grn_po_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.grn
    ADD CONSTRAINT grn_po_id_fkey FOREIGN KEY (po_id) REFERENCES public.purchase_order(id);


--
-- Name: grn grn_received_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.grn
    ADD CONSTRAINT grn_received_by_fkey FOREIGN KEY (received_by) REFERENCES public.users(id);


--
-- Name: grn grn_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.grn
    ADD CONSTRAINT grn_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- Name: grn grn_warehouse_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.grn
    ADD CONSTRAINT grn_warehouse_code_fkey FOREIGN KEY (warehouse_code) REFERENCES public.warehouse(warehouse_code);


--
-- Name: ic_po_receive_document ic_po_receive_document_po_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_po_receive_document
    ADD CONSTRAINT ic_po_receive_document_po_id_fkey FOREIGN KEY (po_id) REFERENCES public.purchase_order(id);


--
-- Name: ic_po_return_document ic_po_return_document_po_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_po_return_document
    ADD CONSTRAINT ic_po_return_document_po_id_fkey FOREIGN KEY (po_id) REFERENCES public.purchase_order(id);


--
-- Name: ic_po_return_line ic_po_return_line_cost_subgroup_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_po_return_line
    ADD CONSTRAINT ic_po_return_line_cost_subgroup_id_fkey FOREIGN KEY (cost_subgroup_id) REFERENCES public.cost_subgroup(id);


--
-- Name: ic_po_return_line ic_po_return_line_po_line_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_po_return_line
    ADD CONSTRAINT ic_po_return_line_po_line_id_fkey FOREIGN KEY (po_line_id) REFERENCES public.purchase_order_line(id);


--
-- Name: ic_po_return_line ic_po_return_line_return_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_po_return_line
    ADD CONSTRAINT ic_po_return_line_return_id_fkey FOREIGN KEY (return_id) REFERENCES public.ic_po_return_document(id);


--
-- Name: ic_project_cost_item ic_project_cost_item_cost_subgroup_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_project_cost_item
    ADD CONSTRAINT ic_project_cost_item_cost_subgroup_id_fkey FOREIGN KEY (cost_subgroup_id) REFERENCES public.cost_subgroup(id);


--
-- Name: ic_project_cost_item_transaction ic_project_cost_item_transaction_movement_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_project_cost_item_transaction
    ADD CONSTRAINT ic_project_cost_item_transaction_movement_id_fkey FOREIGN KEY (movement_id) REFERENCES public.ic_project_movement(id);


--
-- Name: ic_project_cost_item_transaction ic_project_cost_item_transaction_movement_line_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_project_cost_item_transaction
    ADD CONSTRAINT ic_project_cost_item_transaction_movement_line_id_fkey FOREIGN KEY (movement_line_id) REFERENCES public.ic_project_movement_line(id);


--
-- Name: ic_project_cost_item_transaction ic_project_cost_item_transaction_po_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_project_cost_item_transaction
    ADD CONSTRAINT ic_project_cost_item_transaction_po_id_fkey FOREIGN KEY (po_id) REFERENCES public.purchase_order(id);


--
-- Name: ic_project_cost_item_transaction ic_project_cost_item_transaction_po_line_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_project_cost_item_transaction
    ADD CONSTRAINT ic_project_cost_item_transaction_po_line_id_fkey FOREIGN KEY (po_line_id) REFERENCES public.purchase_order_line(id);


--
-- Name: ic_project_cost_item_transaction ic_project_cost_item_transaction_pr_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_project_cost_item_transaction
    ADD CONSTRAINT ic_project_cost_item_transaction_pr_id_fkey FOREIGN KEY (pr_id) REFERENCES public.purchase_request(id);


--
-- Name: ic_project_cost_item_transaction ic_project_cost_item_transaction_pr_line_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_project_cost_item_transaction
    ADD CONSTRAINT ic_project_cost_item_transaction_pr_line_id_fkey FOREIGN KEY (pr_line_id) REFERENCES public.purchase_request_line(id);


--
-- Name: ic_project_cost_item_transaction ic_project_cost_item_transaction_project_cost_item_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_project_cost_item_transaction
    ADD CONSTRAINT ic_project_cost_item_transaction_project_cost_item_id_fkey FOREIGN KEY (project_cost_item_id) REFERENCES public.ic_project_cost_item(id);


--
-- Name: ic_project_cost_item_transaction ic_project_cost_item_transaction_receive_document_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_project_cost_item_transaction
    ADD CONSTRAINT ic_project_cost_item_transaction_receive_document_id_fkey FOREIGN KEY (receive_document_id) REFERENCES public.ic_po_receive_document(id);


--
-- Name: ic_project_cost_item_transaction ic_project_cost_item_transaction_return_document_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_project_cost_item_transaction
    ADD CONSTRAINT ic_project_cost_item_transaction_return_document_id_fkey FOREIGN KEY (return_document_id) REFERENCES public.ic_po_return_document(id);


--
-- Name: ic_project_movement_line ic_project_movement_line_cost_subgroup_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_project_movement_line
    ADD CONSTRAINT ic_project_movement_line_cost_subgroup_id_fkey FOREIGN KEY (cost_subgroup_id) REFERENCES public.cost_subgroup(id);


--
-- Name: ic_project_movement_line ic_project_movement_line_mat_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_project_movement_line
    ADD CONSTRAINT ic_project_movement_line_mat_code_fkey FOREIGN KEY (mat_code) REFERENCES public.material_code(mat_code);


--
-- Name: ic_project_movement_line ic_project_movement_line_movement_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_project_movement_line
    ADD CONSTRAINT ic_project_movement_line_movement_id_fkey FOREIGN KEY (movement_id) REFERENCES public.ic_project_movement(id);


--
-- Name: ic_project_movement_line ic_project_movement_line_to_cost_subgroup_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_project_movement_line
    ADD CONSTRAINT ic_project_movement_line_to_cost_subgroup_id_fkey FOREIGN KEY (to_cost_subgroup_id) REFERENCES public.cost_subgroup(id);


--
-- Name: ic_project_movement_line ic_project_movement_line_to_project_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_project_movement_line
    ADD CONSTRAINT ic_project_movement_line_to_project_code_fkey FOREIGN KEY (to_project_code) REFERENCES public.project(project_code);


--
-- Name: ic_project_movement ic_project_movement_project_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_project_movement
    ADD CONSTRAINT ic_project_movement_project_code_fkey FOREIGN KEY (project_code) REFERENCES public.project(project_code);


--
-- Name: ic_project_movement ic_project_movement_requested_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_project_movement
    ADD CONSTRAINT ic_project_movement_requested_by_fkey FOREIGN KEY (requested_by) REFERENCES public.users(id);


--
-- Name: ic_project_receipt_pool ic_project_receipt_pool_cost_subgroup_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_project_receipt_pool
    ADD CONSTRAINT ic_project_receipt_pool_cost_subgroup_id_fkey FOREIGN KEY (cost_subgroup_id) REFERENCES public.cost_subgroup(id);


--
-- Name: ic_project_receipt_pool ic_project_receipt_pool_mat_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_project_receipt_pool
    ADD CONSTRAINT ic_project_receipt_pool_mat_code_fkey FOREIGN KEY (mat_code) REFERENCES public.material_code(mat_code);


--
-- Name: ic_project_receipt_pool ic_project_receipt_pool_project_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_project_receipt_pool
    ADD CONSTRAINT ic_project_receipt_pool_project_code_fkey FOREIGN KEY (project_code) REFERENCES public.project(project_code);


--
-- Name: ic_project_receipt_pool_transaction ic_project_receipt_pool_transaction_movement_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_project_receipt_pool_transaction
    ADD CONSTRAINT ic_project_receipt_pool_transaction_movement_id_fkey FOREIGN KEY (movement_id) REFERENCES public.ic_project_movement(id);


--
-- Name: ic_project_receipt_pool_transaction ic_project_receipt_pool_transaction_movement_line_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_project_receipt_pool_transaction
    ADD CONSTRAINT ic_project_receipt_pool_transaction_movement_line_id_fkey FOREIGN KEY (movement_line_id) REFERENCES public.ic_project_movement_line(id);


--
-- Name: ic_project_receipt_pool_transaction ic_project_receipt_pool_transaction_po_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_project_receipt_pool_transaction
    ADD CONSTRAINT ic_project_receipt_pool_transaction_po_id_fkey FOREIGN KEY (po_id) REFERENCES public.purchase_order(id);


--
-- Name: ic_project_receipt_pool_transaction ic_project_receipt_pool_transaction_po_line_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_project_receipt_pool_transaction
    ADD CONSTRAINT ic_project_receipt_pool_transaction_po_line_id_fkey FOREIGN KEY (po_line_id) REFERENCES public.purchase_order_line(id);


--
-- Name: ic_project_receipt_pool_transaction ic_project_receipt_pool_transaction_receipt_pool_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_project_receipt_pool_transaction
    ADD CONSTRAINT ic_project_receipt_pool_transaction_receipt_pool_id_fkey FOREIGN KEY (receipt_pool_id) REFERENCES public.ic_project_receipt_pool(id);


--
-- Name: ic_project_receipt_pool_transaction ic_project_receipt_pool_transaction_receive_document_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ic_project_receipt_pool_transaction
    ADD CONSTRAINT ic_project_receipt_pool_transaction_receive_document_id_fkey FOREIGN KEY (receive_document_id) REFERENCES public.ic_po_receive_document(id);


--
-- Name: inventory inventory_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inventory
    ADD CONSTRAINT inventory_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: inventory inventory_mat_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inventory
    ADD CONSTRAINT inventory_mat_code_fkey FOREIGN KEY (mat_code) REFERENCES public.material_code(mat_code) DEFERRABLE;


--
-- Name: inventory_transaction inventory_transaction_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inventory_transaction
    ADD CONSTRAINT inventory_transaction_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: inventory_transaction inventory_transaction_from_warehouse_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inventory_transaction
    ADD CONSTRAINT inventory_transaction_from_warehouse_fkey FOREIGN KEY (from_warehouse) REFERENCES public.warehouse(warehouse_code);


--
-- Name: inventory_transaction inventory_transaction_from_zone_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inventory_transaction
    ADD CONSTRAINT inventory_transaction_from_zone_id_fkey FOREIGN KEY (from_zone_id) REFERENCES public.storage_zone(id);


--
-- Name: inventory_transaction inventory_transaction_location_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inventory_transaction
    ADD CONSTRAINT inventory_transaction_location_code_fkey FOREIGN KEY (location_code) REFERENCES public.location(location_code);


--
-- Name: inventory_transaction inventory_transaction_mat_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inventory_transaction
    ADD CONSTRAINT inventory_transaction_mat_code_fkey FOREIGN KEY (mat_code) REFERENCES public.material_code(mat_code) DEFERRABLE;


--
-- Name: inventory_transaction inventory_transaction_to_warehouse_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inventory_transaction
    ADD CONSTRAINT inventory_transaction_to_warehouse_fkey FOREIGN KEY (to_warehouse) REFERENCES public.warehouse(warehouse_code);


--
-- Name: inventory_transaction inventory_transaction_to_zone_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inventory_transaction
    ADD CONSTRAINT inventory_transaction_to_zone_id_fkey FOREIGN KEY (to_zone_id) REFERENCES public.storage_zone(id);


--
-- Name: inventory_transaction inventory_transaction_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inventory_transaction
    ADD CONSTRAINT inventory_transaction_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- Name: inventory inventory_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inventory
    ADD CONSTRAINT inventory_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- Name: inventory inventory_warehouse_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inventory
    ADD CONSTRAINT inventory_warehouse_code_fkey FOREIGN KEY (warehouse_code) REFERENCES public.warehouse(warehouse_code);


--
-- Name: inventory inventory_zone_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inventory
    ADD CONSTRAINT inventory_zone_id_fkey FOREIGN KEY (zone_id) REFERENCES public.storage_zone(id);


--
-- Name: location location_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.location
    ADD CONSTRAINT location_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: location location_parent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.location
    ADD CONSTRAINT location_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES public.location(id);


--
-- Name: location location_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.location
    ADD CONSTRAINT location_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- Name: location location_warehouse_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.location
    ADD CONSTRAINT location_warehouse_code_fkey FOREIGN KEY (warehouse_code) REFERENCES public.warehouse(warehouse_code);


--
-- Name: mat_group mat_group_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mat_group
    ADD CONSTRAINT mat_group_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: mat_group mat_group_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mat_group
    ADD CONSTRAINT mat_group_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- Name: mat_name mat_name_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mat_name
    ADD CONSTRAINT mat_name_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: mat_name mat_name_subgroup_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mat_name
    ADD CONSTRAINT mat_name_subgroup_id_fkey FOREIGN KEY (subgroup_id) REFERENCES public.subgroup(id);


--
-- Name: mat_name mat_name_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mat_name
    ADD CONSTRAINT mat_name_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- Name: material_code material_code_brand_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.material_code
    ADD CONSTRAINT material_code_brand_id_fkey FOREIGN KEY (brand_id) REFERENCES public.brand(id);


--
-- Name: material_code material_code_cost_subgroup_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.material_code
    ADD CONSTRAINT material_code_cost_subgroup_id_fkey FOREIGN KEY (cost_subgroup_id) REFERENCES public.cost_subgroup(id);


--
-- Name: material_code material_code_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.material_code
    ADD CONSTRAINT material_code_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: material_code material_code_group_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.material_code
    ADD CONSTRAINT material_code_group_id_fkey FOREIGN KEY (group_id) REFERENCES public.mat_group(id);


--
-- Name: material_code material_code_mat_name_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.material_code
    ADD CONSTRAINT material_code_mat_name_id_fkey FOREIGN KEY (mat_name_id) REFERENCES public.mat_name(id);


--
-- Name: material_code material_code_spec_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.material_code
    ADD CONSTRAINT material_code_spec_id_fkey FOREIGN KEY (spec_id) REFERENCES public.spec_size(id);


--
-- Name: material_code material_code_subgroup_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.material_code
    ADD CONSTRAINT material_code_subgroup_id_fkey FOREIGN KEY (subgroup_id) REFERENCES public.subgroup(id);


--
-- Name: material_code material_code_unit_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.material_code
    ADD CONSTRAINT material_code_unit_id_fkey FOREIGN KEY (unit_id) REFERENCES public.unit(id);


--
-- Name: material_code material_code_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.material_code
    ADD CONSTRAINT material_code_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- Name: memo memo_approver_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.memo
    ADD CONSTRAINT memo_approver_id_fk FOREIGN KEY (approver_id) REFERENCES public.users(id);


--
-- Name: memo_attachment memo_attachment_memo_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.memo_attachment
    ADD CONSTRAINT memo_attachment_memo_fk FOREIGN KEY (memo_id) REFERENCES public.memo(id) ON DELETE CASCADE;


--
-- Name: memo_attachment memo_attachment_user_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.memo_attachment
    ADD CONSTRAINT memo_attachment_user_fk FOREIGN KEY (uploaded_by) REFERENCES public.users(id);


--
-- Name: memo_line memo_line_memo_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.memo_line
    ADD CONSTRAINT memo_line_memo_id_fk FOREIGN KEY (memo_id) REFERENCES public.memo(id) ON DELETE CASCADE;


--
-- Name: memo_status_log memo_status_log_memo_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.memo_status_log
    ADD CONSTRAINT memo_status_log_memo_fk FOREIGN KEY (memo_id) REFERENCES public.memo(id) ON DELETE CASCADE;


--
-- Name: memo_status_log memo_status_log_user_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.memo_status_log
    ADD CONSTRAINT memo_status_log_user_fk FOREIGN KEY (changed_by) REFERENCES public.users(id);


--
-- Name: menus menus_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.menus
    ADD CONSTRAINT menus_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: menus menus_parent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.menus
    ADD CONSTRAINT menus_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES public.menus(id) ON DELETE CASCADE;


--
-- Name: menus menus_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.menus
    ADD CONSTRAINT menus_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- Name: modules modules_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.modules
    ADD CONSTRAINT modules_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: modules modules_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.modules
    ADD CONSTRAINT modules_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- Name: payment_installment payment_installment_paid_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payment_installment
    ADD CONSTRAINT payment_installment_paid_by_fkey FOREIGN KEY (paid_by) REFERENCES public.users(id);


--
-- Name: payment_installment payment_installment_payment_log_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payment_installment
    ADD CONSTRAINT payment_installment_payment_log_id_fkey FOREIGN KEY (payment_log_id) REFERENCES public.payment_log(id);


--
-- Name: payment_log payment_log_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payment_log
    ADD CONSTRAINT payment_log_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: payment_log payment_log_paid_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payment_log
    ADD CONSTRAINT payment_log_paid_by_fkey FOREIGN KEY (paid_by) REFERENCES public.users(id);


--
-- Name: payment_log payment_log_reverses_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payment_log
    ADD CONSTRAINT payment_log_reverses_id_fkey FOREIGN KEY (reverses_id) REFERENCES public.payment_log(id);


--
-- Name: permission_audit_logs permission_audit_logs_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.permission_audit_logs
    ADD CONSTRAINT permission_audit_logs_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: permissions permissions_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.permissions
    ADD CONSTRAINT permissions_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: permissions permissions_module_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.permissions
    ADD CONSTRAINT permissions_module_id_fkey FOREIGN KEY (module_id) REFERENCES public.modules(id) ON DELETE CASCADE;


--
-- Name: permissions permissions_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.permissions
    ADD CONSTRAINT permissions_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- Name: petty_cash_attachment petty_cash_attachment_pc_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.petty_cash_attachment
    ADD CONSTRAINT petty_cash_attachment_pc_id_fkey FOREIGN KEY (pc_id) REFERENCES public.petty_cash_requisition(id) ON DELETE CASCADE;


--
-- Name: petty_cash_attachment petty_cash_attachment_uploaded_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.petty_cash_attachment
    ADD CONSTRAINT petty_cash_attachment_uploaded_by_fkey FOREIGN KEY (uploaded_by) REFERENCES public.users(id);


--
-- Name: petty_cash_requisition petty_cash_requisition_approver_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.petty_cash_requisition
    ADD CONSTRAINT petty_cash_requisition_approver_id_fkey FOREIGN KEY (approver_id) REFERENCES public.users(id);


--
-- Name: petty_cash_requisition_line petty_cash_requisition_line_cost_subgroup_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.petty_cash_requisition_line
    ADD CONSTRAINT petty_cash_requisition_line_cost_subgroup_id_fkey FOREIGN KEY (cost_subgroup_id) REFERENCES public.cost_subgroup(id);


--
-- Name: petty_cash_requisition_line petty_cash_requisition_line_mat_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.petty_cash_requisition_line
    ADD CONSTRAINT petty_cash_requisition_line_mat_code_fkey FOREIGN KEY (mat_code) REFERENCES public.material_code(mat_code);


--
-- Name: petty_cash_requisition_line petty_cash_requisition_line_pc_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.petty_cash_requisition_line
    ADD CONSTRAINT petty_cash_requisition_line_pc_id_fkey FOREIGN KEY (pc_id) REFERENCES public.petty_cash_requisition(id) ON DELETE CASCADE;


--
-- Name: petty_cash_requisition petty_cash_requisition_project_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.petty_cash_requisition
    ADD CONSTRAINT petty_cash_requisition_project_code_fkey FOREIGN KEY (project_code) REFERENCES public.project(project_code);


--
-- Name: petty_cash_requisition petty_cash_requisition_requested_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.petty_cash_requisition
    ADD CONSTRAINT petty_cash_requisition_requested_by_fkey FOREIGN KEY (requested_by) REFERENCES public.users(id);


--
-- Name: petty_cash_status_log petty_cash_status_log_changed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.petty_cash_status_log
    ADD CONSTRAINT petty_cash_status_log_changed_by_fkey FOREIGN KEY (changed_by) REFERENCES public.users(id);


--
-- Name: petty_cash_status_log petty_cash_status_log_pc_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.petty_cash_status_log
    ADD CONSTRAINT petty_cash_status_log_pc_id_fkey FOREIGN KEY (pc_id) REFERENCES public.petty_cash_requisition(id) ON DELETE CASCADE;


--
-- Name: po_attachment po_attachment_po_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.po_attachment
    ADD CONSTRAINT po_attachment_po_fk FOREIGN KEY (po_id) REFERENCES public.purchase_order(id);


--
-- Name: po_attachment po_attachment_uploaded_by_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.po_attachment
    ADD CONSTRAINT po_attachment_uploaded_by_fk FOREIGN KEY (uploaded_by) REFERENCES public.users(id);


--
-- Name: po_edit_log po_edit_log_po_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.po_edit_log
    ADD CONSTRAINT po_edit_log_po_fk FOREIGN KEY (po_id) REFERENCES public.purchase_order(id);


--
-- Name: po_edit_log po_edit_log_user_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.po_edit_log
    ADD CONSTRAINT po_edit_log_user_fk FOREIGN KEY (edited_by) REFERENCES public.users(id);


--
-- Name: po_status_log po_status_log_changed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.po_status_log
    ADD CONSTRAINT po_status_log_changed_by_fkey FOREIGN KEY (changed_by) REFERENCES public.users(id);


--
-- Name: po_status_log po_status_log_po_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.po_status_log
    ADD CONSTRAINT po_status_log_po_id_fkey FOREIGN KEY (po_id) REFERENCES public.purchase_order(id);


--
-- Name: purchase_request pr_memo_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_request
    ADD CONSTRAINT pr_memo_id_fk FOREIGN KEY (memo_id) REFERENCES public.memo(id) ON DELETE SET NULL;


--
-- Name: pr_status_log pr_status_log_changed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pr_status_log
    ADD CONSTRAINT pr_status_log_changed_by_fkey FOREIGN KEY (changed_by) REFERENCES public.users(id);


--
-- Name: pr_status_log pr_status_log_pr_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pr_status_log
    ADD CONSTRAINT pr_status_log_pr_id_fkey FOREIGN KEY (pr_id) REFERENCES public.purchase_request(id);


--
-- Name: project_code_counter project_code_counter_cus_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.project_code_counter
    ADD CONSTRAINT project_code_counter_cus_id_fkey FOREIGN KEY (cus_id) REFERENCES public.customer(cus_id);


--
-- Name: project_cost_budget project_cost_budget_cost_subgroup_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.project_cost_budget
    ADD CONSTRAINT project_cost_budget_cost_subgroup_id_fkey FOREIGN KEY (cost_subgroup_id) REFERENCES public.cost_subgroup(id);


--
-- Name: project_cost_budget project_cost_budget_project_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.project_cost_budget
    ADD CONSTRAINT project_cost_budget_project_id_fkey FOREIGN KEY (project_id) REFERENCES public.project(id) ON DELETE CASCADE;


--
-- Name: project project_customer_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.project
    ADD CONSTRAINT project_customer_id_fkey FOREIGN KEY (customer_id) REFERENCES public.customer(cus_id);


--
-- Name: project project_dept_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.project
    ADD CONSTRAINT project_dept_code_fkey FOREIGN KEY (dept_code) REFERENCES public.departments(dept_code) ON UPDATE CASCADE;


--
-- Name: project_job_share project_job_share_project_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.project_job_share
    ADD CONSTRAINT project_job_share_project_id_fkey FOREIGN KEY (project_id) REFERENCES public.project(id) ON DELETE CASCADE;


--
-- Name: project project_linked_warehouse_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.project
    ADD CONSTRAINT project_linked_warehouse_code_fkey FOREIGN KEY (linked_warehouse_code) REFERENCES public.warehouse(warehouse_code);


--
-- Name: project project_owner_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.project
    ADD CONSTRAINT project_owner_id_fk FOREIGN KEY (owner_id) REFERENCES public.users(id);


--
-- Name: project_responsible_person project_responsible_person_project_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.project_responsible_person
    ADD CONSTRAINT project_responsible_person_project_id_fkey FOREIGN KEY (project_id) REFERENCES public.project(id) ON DELETE CASCADE;


--
-- Name: project_responsible_person project_responsible_person_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.project_responsible_person
    ADD CONSTRAINT project_responsible_person_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: purchase_order purchase_order_approver_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_order
    ADD CONSTRAINT purchase_order_approver_fk FOREIGN KEY (approver_id) REFERENCES public.users(id);


--
-- Name: purchase_order purchase_order_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_order
    ADD CONSTRAINT purchase_order_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: purchase_order purchase_order_deposit_paid_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_order
    ADD CONSTRAINT purchase_order_deposit_paid_by_fkey FOREIGN KEY (deposit_paid_by) REFERENCES public.users(id);


--
-- Name: purchase_order_line purchase_order_line_cost_subgroup_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_order_line
    ADD CONSTRAINT purchase_order_line_cost_subgroup_id_fkey FOREIGN KEY (cost_subgroup_id) REFERENCES public.cost_subgroup(id);


--
-- Name: purchase_order_line purchase_order_line_mat_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_order_line
    ADD CONSTRAINT purchase_order_line_mat_code_fkey FOREIGN KEY (mat_code) REFERENCES public.material_code(mat_code) DEFERRABLE;


--
-- Name: purchase_order_line purchase_order_line_po_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_order_line
    ADD CONSTRAINT purchase_order_line_po_id_fkey FOREIGN KEY (po_id) REFERENCES public.purchase_order(id) ON DELETE CASCADE;


--
-- Name: purchase_order_line purchase_order_line_pr_line_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_order_line
    ADD CONSTRAINT purchase_order_line_pr_line_id_fkey FOREIGN KEY (pr_line_id) REFERENCES public.purchase_request_line(id);


--
-- Name: purchase_order purchase_order_location_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_order
    ADD CONSTRAINT purchase_order_location_fk FOREIGN KEY (location_code) REFERENCES public.location(location_code);


--
-- Name: purchase_order purchase_order_pr_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_order
    ADD CONSTRAINT purchase_order_pr_id_fkey FOREIGN KEY (pr_id) REFERENCES public.purchase_request(id);


--
-- Name: purchase_order purchase_order_project_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_order
    ADD CONSTRAINT purchase_order_project_fk FOREIGN KEY (project_code) REFERENCES public.project(project_code);


--
-- Name: purchase_order purchase_order_requested_by_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_order
    ADD CONSTRAINT purchase_order_requested_by_fk FOREIGN KEY (requested_by) REFERENCES public.users(id);


--
-- Name: purchase_order purchase_order_rfq_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_order
    ADD CONSTRAINT purchase_order_rfq_id_fkey FOREIGN KEY (rfq_id) REFERENCES public.rfq(id);


--
-- Name: purchase_order purchase_order_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_order
    ADD CONSTRAINT purchase_order_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- Name: purchase_order purchase_order_warehouse_code_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_order
    ADD CONSTRAINT purchase_order_warehouse_code_fk FOREIGN KEY (warehouse_code) REFERENCES public.warehouse(warehouse_code) ON DELETE SET NULL;


--
-- Name: purchase_request purchase_request_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_request
    ADD CONSTRAINT purchase_request_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: purchase_request_line purchase_request_line_cost_subgroup_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_request_line
    ADD CONSTRAINT purchase_request_line_cost_subgroup_fk FOREIGN KEY (cost_subgroup_id) REFERENCES public.cost_subgroup(id);


--
-- Name: purchase_request_line purchase_request_line_mat_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_request_line
    ADD CONSTRAINT purchase_request_line_mat_code_fkey FOREIGN KEY (mat_code) REFERENCES public.material_code(mat_code) DEFERRABLE;


--
-- Name: purchase_request_line purchase_request_line_memo_line_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_request_line
    ADD CONSTRAINT purchase_request_line_memo_line_id_fkey FOREIGN KEY (memo_line_id) REFERENCES public.memo_line(id);


--
-- Name: purchase_request_line purchase_request_line_pr_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_request_line
    ADD CONSTRAINT purchase_request_line_pr_id_fkey FOREIGN KEY (pr_id) REFERENCES public.purchase_request(id) ON DELETE CASCADE;


--
-- Name: purchase_request purchase_request_requested_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_request
    ADD CONSTRAINT purchase_request_requested_by_fkey FOREIGN KEY (requested_by) REFERENCES public.users(id);


--
-- Name: purchase_request purchase_request_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_request
    ADD CONSTRAINT purchase_request_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- Name: purchase_request purchase_request_warehouse_code_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_request
    ADD CONSTRAINT purchase_request_warehouse_code_fk FOREIGN KEY (warehouse_code) REFERENCES public.warehouse(warehouse_code) ON DELETE SET NULL;


--
-- Name: purchase_request purchase_request_warehouse_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_request
    ADD CONSTRAINT purchase_request_warehouse_code_fkey FOREIGN KEY (warehouse_code) REFERENCES public.warehouse(warehouse_code);


--
-- Name: requisition requisition_checked_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.requisition
    ADD CONSTRAINT requisition_checked_by_fkey FOREIGN KEY (checked_by) REFERENCES public.users(id);


--
-- Name: requisition_line requisition_line_req_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.requisition_line
    ADD CONSTRAINT requisition_line_req_id_fkey FOREIGN KEY (req_id) REFERENCES public.requisition(id);


--
-- Name: requisition_line requisition_line_stock_item_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.requisition_line
    ADD CONSTRAINT requisition_line_stock_item_id_fkey FOREIGN KEY (stock_item_id) REFERENCES public.stock_item(id);


--
-- Name: requisition requisition_project_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.requisition
    ADD CONSTRAINT requisition_project_code_fkey FOREIGN KEY (project_code) REFERENCES public.project(project_code);


--
-- Name: requisition requisition_requester_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.requisition
    ADD CONSTRAINT requisition_requester_id_fkey FOREIGN KEY (requester_id) REFERENCES public.users(id);


--
-- Name: requisition_status_log requisition_status_log_changed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.requisition_status_log
    ADD CONSTRAINT requisition_status_log_changed_by_fkey FOREIGN KEY (changed_by) REFERENCES public.users(id);


--
-- Name: requisition_status_log requisition_status_log_req_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.requisition_status_log
    ADD CONSTRAINT requisition_status_log_req_id_fkey FOREIGN KEY (req_id) REFERENCES public.requisition(id);


--
-- Name: rfq rfq_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rfq
    ADD CONSTRAINT rfq_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: rfq_line rfq_line_mat_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rfq_line
    ADD CONSTRAINT rfq_line_mat_code_fkey FOREIGN KEY (mat_code) REFERENCES public.material_code(mat_code) DEFERRABLE;


--
-- Name: rfq_line rfq_line_rfq_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rfq_line
    ADD CONSTRAINT rfq_line_rfq_id_fkey FOREIGN KEY (rfq_id) REFERENCES public.rfq(id) ON DELETE CASCADE;


--
-- Name: rfq rfq_pr_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rfq
    ADD CONSTRAINT rfq_pr_id_fkey FOREIGN KEY (pr_id) REFERENCES public.purchase_request(id);


--
-- Name: rfq rfq_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rfq
    ADD CONSTRAINT rfq_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- Name: role_menu_permissions role_menu_permissions_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.role_menu_permissions
    ADD CONSTRAINT role_menu_permissions_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: role_menu_permissions role_menu_permissions_menu_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.role_menu_permissions
    ADD CONSTRAINT role_menu_permissions_menu_id_fkey FOREIGN KEY (menu_id) REFERENCES public.menus(id) ON DELETE CASCADE;


--
-- Name: role_menu_permissions role_menu_permissions_role_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.role_menu_permissions
    ADD CONSTRAINT role_menu_permissions_role_id_fkey FOREIGN KEY (role_id) REFERENCES public.roles(id) ON DELETE CASCADE;


--
-- Name: role_menu_permissions role_menu_permissions_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.role_menu_permissions
    ADD CONSTRAINT role_menu_permissions_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- Name: role_menus role_menus_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.role_menus
    ADD CONSTRAINT role_menus_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: role_menus role_menus_menu_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.role_menus
    ADD CONSTRAINT role_menus_menu_id_fkey FOREIGN KEY (menu_id) REFERENCES public.menus(id) ON DELETE CASCADE;


--
-- Name: role_menus role_menus_role_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.role_menus
    ADD CONSTRAINT role_menus_role_id_fkey FOREIGN KEY (role_id) REFERENCES public.roles(id) ON DELETE CASCADE;


--
-- Name: role_permissions role_permissions_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.role_permissions
    ADD CONSTRAINT role_permissions_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: role_permissions role_permissions_permission_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.role_permissions
    ADD CONSTRAINT role_permissions_permission_id_fkey FOREIGN KEY (permission_id) REFERENCES public.permissions(id) ON DELETE CASCADE;


--
-- Name: role_permissions role_permissions_role_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.role_permissions
    ADD CONSTRAINT role_permissions_role_id_fkey FOREIGN KEY (role_id) REFERENCES public.roles(id) ON DELETE CASCADE;


--
-- Name: roles roles_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.roles
    ADD CONSTRAINT roles_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: roles roles_dept_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.roles
    ADD CONSTRAINT roles_dept_code_fkey FOREIGN KEY (dept_code) REFERENCES public.departments(dept_code) ON UPDATE CASCADE;


--
-- Name: roles roles_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.roles
    ADD CONSTRAINT roles_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- Name: spec_size spec_size_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.spec_size
    ADD CONSTRAINT spec_size_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: spec_size spec_size_mat_name_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.spec_size
    ADD CONSTRAINT spec_size_mat_name_id_fkey FOREIGN KEY (mat_name_id) REFERENCES public.mat_name(id);


--
-- Name: spec_size spec_size_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.spec_size
    ADD CONSTRAINT spec_size_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- Name: stock_count stock_count_approved_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_count
    ADD CONSTRAINT stock_count_approved_by_fkey FOREIGN KEY (approved_by) REFERENCES public.users(id);


--
-- Name: stock_count stock_count_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_count
    ADD CONSTRAINT stock_count_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: stock_count_line stock_count_line_count_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_count_line
    ADD CONSTRAINT stock_count_line_count_id_fkey FOREIGN KEY (count_id) REFERENCES public.stock_count(id) ON DELETE CASCADE;


--
-- Name: stock_count_line stock_count_line_mat_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_count_line
    ADD CONSTRAINT stock_count_line_mat_code_fkey FOREIGN KEY (mat_code) REFERENCES public.material_code(mat_code) DEFERRABLE;


--
-- Name: stock_count_line stock_count_line_zone_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_count_line
    ADD CONSTRAINT stock_count_line_zone_id_fkey FOREIGN KEY (zone_id) REFERENCES public.storage_zone(id);


--
-- Name: stock_count stock_count_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_count
    ADD CONSTRAINT stock_count_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- Name: stock_count stock_count_warehouse_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_count
    ADD CONSTRAINT stock_count_warehouse_code_fkey FOREIGN KEY (warehouse_code) REFERENCES public.warehouse(warehouse_code);


--
-- Name: stock_inventory stock_inventory_item_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_inventory
    ADD CONSTRAINT stock_inventory_item_fk FOREIGN KEY (item_id) REFERENCES public.stock_item(id);


--
-- Name: stock_inventory stock_inventory_loc_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_inventory
    ADD CONSTRAINT stock_inventory_loc_fk FOREIGN KEY (location_code) REFERENCES public.location(location_code);


--
-- Name: stock_item stock_item_category_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_item
    ADD CONSTRAINT stock_item_category_fk FOREIGN KEY (category_id) REFERENCES public.stock_category(id) ON DELETE SET NULL;


--
-- Name: stock_item stock_item_created_by_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_item
    ADD CONSTRAINT stock_item_created_by_fk FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: stock_item_image stock_item_image_item_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_item_image
    ADD CONSTRAINT stock_item_image_item_id_fkey FOREIGN KEY (item_id) REFERENCES public.stock_item(id) ON DELETE CASCADE;


--
-- Name: stock_item stock_item_location_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_item
    ADD CONSTRAINT stock_item_location_code_fkey FOREIGN KEY (location_code) REFERENCES public.location(location_code);


--
-- Name: stock_item stock_item_updated_by_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_item
    ADD CONSTRAINT stock_item_updated_by_fk FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- Name: stock_reservation stock_res_item_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_reservation
    ADD CONSTRAINT stock_res_item_fk FOREIGN KEY (item_id) REFERENCES public.stock_item(id);


--
-- Name: stock_reservation stock_res_loc_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_reservation
    ADD CONSTRAINT stock_res_loc_fk FOREIGN KEY (location_code) REFERENCES public.location(location_code);


--
-- Name: stock_reservation stock_res_user_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_reservation
    ADD CONSTRAINT stock_res_user_fk FOREIGN KEY (requested_by) REFERENCES public.users(id);


--
-- Name: stock_transaction stock_transaction_receive_document_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_transaction
    ADD CONSTRAINT stock_transaction_receive_document_id_fkey FOREIGN KEY (receive_document_id) REFERENCES public.ic_po_receive_document(id);


--
-- Name: stock_transaction stock_transaction_return_document_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_transaction
    ADD CONSTRAINT stock_transaction_return_document_id_fkey FOREIGN KEY (return_document_id) REFERENCES public.ic_po_return_document(id);


--
-- Name: stock_transfer stock_transfer_checked_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_transfer
    ADD CONSTRAINT stock_transfer_checked_by_fkey FOREIGN KEY (checked_by) REFERENCES public.users(id);


--
-- Name: stock_transfer stock_transfer_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_transfer
    ADD CONSTRAINT stock_transfer_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: stock_transfer_line stock_transfer_line_item_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_transfer_line
    ADD CONSTRAINT stock_transfer_line_item_id_fkey FOREIGN KEY (item_id) REFERENCES public.stock_item(id);


--
-- Name: stock_transfer_line stock_transfer_line_transfer_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_transfer_line
    ADD CONSTRAINT stock_transfer_line_transfer_id_fkey FOREIGN KEY (transfer_id) REFERENCES public.stock_transfer(id);


--
-- Name: stock_transfer stock_transfer_requested_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_transfer
    ADD CONSTRAINT stock_transfer_requested_by_fkey FOREIGN KEY (requested_by) REFERENCES public.users(id);


--
-- Name: stock_transfer_status_log stock_transfer_status_log_changed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_transfer_status_log
    ADD CONSTRAINT stock_transfer_status_log_changed_by_fkey FOREIGN KEY (changed_by) REFERENCES public.users(id);


--
-- Name: stock_transfer_status_log stock_transfer_status_log_transfer_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_transfer_status_log
    ADD CONSTRAINT stock_transfer_status_log_transfer_id_fkey FOREIGN KEY (transfer_id) REFERENCES public.stock_transfer(id);


--
-- Name: stock_transfer stock_transfer_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_transfer
    ADD CONSTRAINT stock_transfer_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- Name: stock_transaction stock_txn_created_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_transaction
    ADD CONSTRAINT stock_txn_created_fk FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: stock_transaction stock_txn_item_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stock_transaction
    ADD CONSTRAINT stock_txn_item_fk FOREIGN KEY (item_id) REFERENCES public.stock_item(id);


--
-- Name: storage_zone storage_zone_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.storage_zone
    ADD CONSTRAINT storage_zone_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: storage_zone storage_zone_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.storage_zone
    ADD CONSTRAINT storage_zone_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- Name: storage_zone storage_zone_warehouse_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.storage_zone
    ADD CONSTRAINT storage_zone_warehouse_id_fkey FOREIGN KEY (warehouse_id) REFERENCES public.warehouse(id);


--
-- Name: subgroup subgroup_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subgroup
    ADD CONSTRAINT subgroup_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: subgroup subgroup_group_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subgroup
    ADD CONSTRAINT subgroup_group_id_fkey FOREIGN KEY (group_id) REFERENCES public.mat_group(id);


--
-- Name: subgroup subgroup_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subgroup
    ADD CONSTRAINT subgroup_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- Name: supplier supplier_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.supplier
    ADD CONSTRAINT supplier_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: supplier supplier_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.supplier
    ADD CONSTRAINT supplier_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- Name: unit unit_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.unit
    ADD CONSTRAINT unit_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: unit unit_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.unit
    ADD CONSTRAINT unit_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- Name: user_menu_permissions user_menu_permissions_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_menu_permissions
    ADD CONSTRAINT user_menu_permissions_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: user_menu_permissions user_menu_permissions_menu_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_menu_permissions
    ADD CONSTRAINT user_menu_permissions_menu_id_fkey FOREIGN KEY (menu_id) REFERENCES public.menus(id) ON DELETE CASCADE;


--
-- Name: user_menu_permissions user_menu_permissions_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_menu_permissions
    ADD CONSTRAINT user_menu_permissions_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- Name: user_menu_permissions user_menu_permissions_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_menu_permissions
    ADD CONSTRAINT user_menu_permissions_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: user_permissions user_permissions_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_permissions
    ADD CONSTRAINT user_permissions_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: user_permissions user_permissions_permission_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_permissions
    ADD CONSTRAINT user_permissions_permission_id_fkey FOREIGN KEY (permission_id) REFERENCES public.permissions(id) ON DELETE CASCADE;


--
-- Name: user_permissions user_permissions_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_permissions
    ADD CONSTRAINT user_permissions_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- Name: user_permissions user_permissions_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_permissions
    ADD CONSTRAINT user_permissions_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: user_roles user_roles_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_roles
    ADD CONSTRAINT user_roles_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: user_roles user_roles_role_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_roles
    ADD CONSTRAINT user_roles_role_id_fkey FOREIGN KEY (role_id) REFERENCES public.roles(id) ON DELETE CASCADE;


--
-- Name: user_roles user_roles_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_roles
    ADD CONSTRAINT user_roles_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: users users_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: users users_dept_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_dept_code_fkey FOREIGN KEY (dept_code) REFERENCES public.departments(dept_code) ON UPDATE CASCADE;


--
-- Name: users users_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- Name: warehouse warehouse_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.warehouse
    ADD CONSTRAINT warehouse_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id);


--
-- Name: warehouse warehouse_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.warehouse
    ADD CONSTRAINT warehouse_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id);


--
-- Name: wo_contractor_log_bill_invoice wo_contractor_log_bill_invoice_bill_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wo_contractor_log_bill_invoice
    ADD CONSTRAINT wo_contractor_log_bill_invoice_bill_id_fkey FOREIGN KEY (bill_id) REFERENCES public.wo_contractor_log_bill(id) ON DELETE CASCADE;


--
-- Name: wo_contractor_log_bill_line wo_contractor_log_bill_line_bill_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wo_contractor_log_bill_line
    ADD CONSTRAINT wo_contractor_log_bill_line_bill_id_fkey FOREIGN KEY (bill_id) REFERENCES public.wo_contractor_log_bill(id) ON DELETE CASCADE;


--
-- Name: wo_contractor_log_bill wo_contractor_log_bill_log_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wo_contractor_log_bill
    ADD CONSTRAINT wo_contractor_log_bill_log_id_fkey FOREIGN KEY (log_id) REFERENCES public.wo_contractor_log(id) ON DELETE CASCADE;


--
-- Name: wo_contractor_log wo_contractor_log_project_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wo_contractor_log
    ADD CONSTRAINT wo_contractor_log_project_code_fkey FOREIGN KEY (project_code) REFERENCES public.project(project_code);


--
-- Name: wo_contractor_log_term wo_contractor_log_term_log_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wo_contractor_log_term
    ADD CONSTRAINT wo_contractor_log_term_log_id_fkey FOREIGN KEY (log_id) REFERENCES public.wo_contractor_log(id) ON DELETE CASCADE;


--
-- Name: wo_contractor_log wo_contractor_log_wo_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wo_contractor_log
    ADD CONSTRAINT wo_contractor_log_wo_id_fkey FOREIGN KEY (wo_id) REFERENCES public.work_order(id);


--
-- Name: work_order work_order_approver_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.work_order
    ADD CONSTRAINT work_order_approver_id_fkey FOREIGN KEY (approver_id) REFERENCES public.users(id);


--
-- Name: work_order_attachment work_order_attachment_wo_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.work_order_attachment
    ADD CONSTRAINT work_order_attachment_wo_id_fkey FOREIGN KEY (wo_id) REFERENCES public.work_order(id);


--
-- Name: work_order_cost_code work_order_cost_code_wo_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.work_order_cost_code
    ADD CONSTRAINT work_order_cost_code_wo_id_fkey FOREIGN KEY (wo_id) REFERENCES public.work_order(id);


--
-- Name: work_order_edit_log work_order_edit_log_edited_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.work_order_edit_log
    ADD CONSTRAINT work_order_edit_log_edited_by_fkey FOREIGN KEY (edited_by) REFERENCES public.users(id);


--
-- Name: work_order_edit_log work_order_edit_log_wo_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.work_order_edit_log
    ADD CONSTRAINT work_order_edit_log_wo_id_fkey FOREIGN KEY (wo_id) REFERENCES public.work_order(id) ON DELETE CASCADE;


--
-- Name: work_order_line work_order_line_wo_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.work_order_line
    ADD CONSTRAINT work_order_line_wo_id_fkey FOREIGN KEY (wo_id) REFERENCES public.work_order(id);


--
-- Name: work_order_payment_installment work_order_payment_installment_wo_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.work_order_payment_installment
    ADD CONSTRAINT work_order_payment_installment_wo_id_fkey FOREIGN KEY (wo_id) REFERENCES public.work_order(id);


--
-- Name: work_order_penalty work_order_penalty_wo_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.work_order_penalty
    ADD CONSTRAINT work_order_penalty_wo_id_fkey FOREIGN KEY (wo_id) REFERENCES public.work_order(id);


--
-- Name: work_order_retention work_order_retention_wo_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.work_order_retention
    ADD CONSTRAINT work_order_retention_wo_id_fkey FOREIGN KEY (wo_id) REFERENCES public.work_order(id);


--
-- Name: work_order_status_log work_order_status_log_wo_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.work_order_status_log
    ADD CONSTRAINT work_order_status_log_wo_id_fkey FOREIGN KEY (wo_id) REFERENCES public.work_order(id);


--
-- PostgreSQL database dump complete
--

\unrestrict El7wLBoi5G9jzH6bRSIEXpa4L1g1HX4PQRwJr3jjtTe2OjNYGxcwbrlUgkvmgrT

